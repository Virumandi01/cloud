package users

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type UserAccount struct {
	ID                string    `json:"id"`
	Username          string    `json:"username"`
	PasscodeHash      string    `json:"passcode_hash"`
	StorageLimitBytes int64     `json:"storage_limit_bytes"`
	UsedBytes         int64     `json:"used_bytes"`
	IsActive          bool      `json:"is_active"`
	Role              string    `json:"role"`
	CreatedAt         time.Time `json:"created_at"`
}

type UserManager struct {
	mu       sync.RWMutex
	filePath string
	dataDir  string
	users    map[string]*UserAccount
}

func NewUserManager(dataDir string) (*UserManager, error) {
	filePath := filepath.Join(dataDir, "users.json")
	um := &UserManager{
		filePath: filePath,
		dataDir:  dataDir,
		users:    make(map[string]*UserAccount),
	}

	if err := um.Load(); err != nil {
		return nil, err
	}
	return um, nil
}
func (um *UserManager) GetUser(username string) (*UserAccount, bool) {
	um.mu.RLock()
	defer um.mu.RUnlock()

	user, exists := um.users[username]
	if !exists {
		return nil, false
	}
	// Return a copy to prevent external mutation
	copyUser := *user
	return &copyUser, true
}
func (um *UserManager) Load() error {
	um.mu.Lock()
	defer um.mu.Unlock()

	data, err := os.ReadFile(um.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var root struct {
		Users []UserAccount `json:"users"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("malformed users.json: %w", err)
	}

	um.users = make(map[string]*UserAccount)
	for _, u := range root.Users {
		copyUser := u
		um.users[u.Username] = &copyUser
		// Ensure user directory layout exists
		userDir := filepath.Join(um.dataDir, "users", u.Username, "files")
		chunkDir := filepath.Join(um.dataDir, "users", u.Username, ".chunks")
		_ = os.MkdirAll(userDir, 0755)
		_ = os.MkdirAll(chunkDir, 0755)
	}

	return nil
}

func (um *UserManager) Save() error {
	um.mu.RLock()
	var list []UserAccount
	for _, u := range um.users {
		list = append(list, *u)
	}
	um.mu.RUnlock()

	raw, err := json.MarshalIndent(struct {
		Users []UserAccount `json:"users"`
	}{Users: list}, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(um.filePath, raw, 0600)
}

func (um *UserManager) Authenticate(username, passcode string) (*UserAccount, error) {
	um.mu.RLock()
	user, exists := um.users[username]
	um.mu.RUnlock()

	if !exists || !user.IsActive {
		return nil, errors.New("invalid credentials or inactive account")
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.PasscodeHash), []byte(passcode))
	if err != nil {
		return nil, errors.New("invalid credentials")
	}

	return user, nil
}

func (um *UserManager) CheckQuota(username string, incomingBytes int64) error {
	um.mu.RLock()
	user, exists := um.users[username]
	um.mu.RUnlock()

	if !exists {
		return errors.New("user not found")
	}

	if user.UsedBytes+incomingBytes > user.StorageLimitBytes {
		return fmt.Errorf("quota exceeded: using %d of %d bytes", user.UsedBytes, user.StorageLimitBytes)
	}
	return nil
}

func (um *UserManager) AddUsedBytes(username string, bytes int64) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	user, exists := um.users[username]
	if !exists {
		return errors.New("user not found")
	}

	user.UsedBytes += bytes
	return um.Save()
}

func (um *UserManager) SetUserQuota(username string, limitBytes int64) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	user, exists := um.users[username]
	if !exists {
		return errors.New("user not found")
	}

	user.StorageLimitBytes = limitBytes
	return um.Save()
}

func (um *UserManager) SetUserPasscode(username, newPasscode string) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	user, exists := um.users[username]
	if !exists {
		return errors.New("user not found")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPasscode), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.PasscodeHash = string(hash)
	return um.Save()
}
