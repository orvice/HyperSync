package auth

import (
	"context"
	"errors"
	"sync"
)

var ErrUserNotFound = errors.New("user not found")

type User struct {
	ID           string
	Username     string
	PasswordHash string
	// TokenVersion is embedded in JWT claims at login; a password change
	// bumps it, invalidating every previously issued token.
	TokenVersion int64
}

type UserStore interface {
	GetByUsername(ctx context.Context, username string) (*User, error)
	Create(ctx context.Context, user *User) error
	// UpdatePassword sets the new hash and bumps TokenVersion in the same
	// operation so outstanding tokens are invalidated with the change.
	UpdatePassword(ctx context.Context, username string, newHash string) error
}

type MemoryUserStore struct {
	mu    sync.RWMutex
	users map[string]*User
}

func NewMemoryUserStore() *MemoryUserStore {
	return &MemoryUserStore{
		users: make(map[string]*User),
	}
}

func (s *MemoryUserStore) GetByUsername(ctx context.Context, username string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[username]
	if !ok {
		return nil, ErrUserNotFound
	}
	return u, nil
}

func (s *MemoryUserStore) Create(ctx context.Context, user *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[user.Username] = user
	return nil
}

func (s *MemoryUserStore) UpdatePassword(ctx context.Context, username string, newHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[username]
	if !ok {
		return ErrUserNotFound
	}
	u.PasswordHash = newHash
	u.TokenVersion++
	return nil
}
