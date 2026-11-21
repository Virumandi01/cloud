package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"

	"cloud-core/internal/users"
)

type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]string // token -> username
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]string),
	}
}

func (s *SessionStore) Create(username string) string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)

	s.mu.Lock()
	s.sessions[token] = username
	s.mu.Unlock()
	return token
}

func (s *SessionStore) GetUser(r *http.Request) (string, bool) {
	cookie, err := r.Cookie("cloud_session")
	if err != nil || cookie.Value == "" {
		return "", false
	}
	s.mu.RLock()
	user, ok := s.sessions[cookie.Value]
	s.mu.RUnlock()
	return user, ok
}

func (s *SessionStore) Delete(r *http.Request) {
	cookie, err := r.Cookie("cloud_session")
	if err == nil {
		s.mu.Lock()
		delete(s.sessions, cookie.Value)
		s.mu.Unlock()
	}
}

// RequireAdmin verifies the active session belongs to a host admin
func (s *SessionStore) RequireAdmin(r *http.Request, um *users.UserManager) (*users.UserAccount, bool) {
	username, ok := s.GetUser(r)
	if !ok {
		return nil, false
	}
	u, exists := um.GetUser(username)
	if !exists || u.Role != "admin" {
		return nil, false
	}
	return u, true
}
