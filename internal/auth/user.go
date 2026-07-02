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
}

type UserStore interface {
	GetByUsername(ctx context.Context, username string) (*User, error)
	Create(ctx context.Context, user *User) error
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
	return nil
}
