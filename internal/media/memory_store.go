package media

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type MemoryStore struct {
	mu     sync.RWMutex
	items  map[string]*Media
	nextID int
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{items: make(map[string]*Media)}
}

func (s *MemoryStore) Create(_ context.Context, m *Media) (*Media, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	m.ID = fmt.Sprintf("%d", s.nextID)
	m.CreatedAt = time.Now()

	stored := *m
	s.items[m.ID] = &stored
	return m, nil
}

func (s *MemoryStore) GetByID(_ context.Context, id string) (*Media, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	copy := *m
	return &copy, nil
}

func (s *MemoryStore) List(_ context.Context, opts ListOptions) (*ListResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	all := make([]*Media, 0, len(s.items))
	for _, m := range s.items {
		copy := *m
		all = append(all, &copy)
	}

	total := len(all)
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	page := opts.Page
	if page <= 0 {
		page = 1
	}

	start := (page - 1) * pageSize
	if start >= total {
		return &ListResult{Items: []*Media{}, Total: total}, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	return &ListResult{Items: all[start:end], Total: total}, nil
}

func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.items[id]; !ok {
		return ErrNotFound
	}
	delete(s.items, id)
	return nil
}
