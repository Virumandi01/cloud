package api

import (
	"bufio"
	"os"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

type AccessList struct {
	mu       sync.RWMutex
	filePath string
	allowed  map[string]bool
}

func NewAccessList(filePath string) *AccessList {
	al := &AccessList{
		filePath: filePath,
		allowed:  make(map[string]bool),
	}
	al.reload()
	go al.watch()
	return al
}

func (al *AccessList) reload() {
	f, err := os.Open(al.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			_, _ = os.Create(al.filePath)
		}
		return
	}
	defer f.Close()

	updated := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			updated[line] = true
		}
	}

	al.mu.Lock()
	al.allowed = updated
	al.mu.Unlock()
}

func (al *AccessList) IsPublic(path string) bool {
	al.mu.RLock()
	defer al.mu.RUnlock()
	return al.allowed[path]
}

func (al *AccessList) watch() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer watcher.Close()

	_ = watcher.Add(al.filePath)
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				al.reload()
			}
		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

// Add marks a file path as public and writes to access.txt
func (al *AccessList) Add(entry string) error {
	al.mu.Lock()
	defer al.mu.Unlock()

	entry = strings.TrimSpace(entry)
	if entry == "" {
		return nil
	}

	al.allowed[entry] = true
	return al.flush()
}

// Remove unmarks a file and removes it from access.txt
func (al *AccessList) Remove(entry string) error {
	al.mu.Lock()
	defer al.mu.Unlock()

	delete(al.allowed, strings.TrimSpace(entry))
	return al.flush()
}

// Toggle flips the public status
func (al *AccessList) Toggle(entry string) (bool, error) {
	al.mu.Lock()
	defer al.mu.Unlock()

	entry = strings.TrimSpace(entry)
	curr := al.allowed[entry]
	if curr {
		delete(al.allowed, entry)
	} else {
		al.allowed[entry] = true
	}

	return !curr, al.flush()
}

// flush writes current allowed state to data/access.txt
func (al *AccessList) flush() error {
	f, err := os.Create(al.filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for item := range al.allowed {
		_, _ = w.WriteString(item + "\n")
	}
	return w.Flush()
}
