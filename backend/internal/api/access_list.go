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
