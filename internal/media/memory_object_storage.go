package media

import (
	"context"
	"io"
	"sync"
)

type MemoryObjectStorage struct {
	mu      sync.RWMutex
	objects map[string][]byte
}

func NewMemoryObjectStorage() *MemoryObjectStorage {
	return &MemoryObjectStorage{objects: make(map[string][]byte)}
}

func (s *MemoryObjectStorage) Upload(_ context.Context, key string, _ string, body io.Reader) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = data
	return nil
}

func (s *MemoryObjectStorage) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

func (s *MemoryObjectStorage) Has(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.objects[key]
	return ok
}
