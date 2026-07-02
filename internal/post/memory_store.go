package post

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type MemoryStore struct {
	mu     sync.RWMutex
	posts  map[string]*Post
	nextID int
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		posts: make(map[string]*Post),
	}
}

func (s *MemoryStore) Create(_ context.Context, p *Post) (*Post, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	p.ID = fmt.Sprintf("%d", s.nextID)
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	if p.CrossPostStatus == nil {
		p.CrossPostStatus = make(map[string]CrossPostStatus)
	}

	stored := *p
	s.posts[p.ID] = &stored
	return p, nil
}

func (s *MemoryStore) GetByID(_ context.Context, id string) (*Post, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.posts[id]
	if !ok {
		return nil, ErrNotFound
	}
	copy := *p
	return &copy, nil
}

func (s *MemoryStore) List(_ context.Context, opts ListOptions) (*ListResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var filtered []*Post
	for _, p := range s.posts {
		if opts.Status != "" && p.Status != opts.Status {
			continue
		}
		copy := *p
		filtered = append(filtered, &copy)
	}

	total := len(filtered)
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
		return &ListResult{Posts: []*Post{}, Total: total}, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	return &ListResult{Posts: filtered[start:end], Total: total}, nil
}

func (s *MemoryStore) Update(_ context.Context, p *Post) (*Post, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.posts[p.ID]
	if !ok {
		return nil, ErrNotFound
	}

	p.CreatedAt = existing.CreatedAt
	p.UpdatedAt = time.Now()
	stored := *p
	s.posts[p.ID] = &stored
	return p, nil
}

func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.posts[id]; !ok {
		return ErrNotFound
	}
	delete(s.posts, id)
	return nil
}

func (s *MemoryStore) ListPendingSync(_ context.Context) ([]*Post, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Post
	for _, p := range s.posts {
		if p.Status != "published" {
			continue
		}
		if len(p.SyncTargets) == 0 {
			continue
		}
		for _, target := range p.SyncTargets {
			status, exists := p.CrossPostStatus[target]
			if !exists || (!status.Success && status.RetryCount < 100) || status.NeedsUpdate {
				copy := *p
				if copy.CrossPostStatus != nil {
					cpsCopy := make(map[string]CrossPostStatus)
					for k, v := range copy.CrossPostStatus {
						cpsCopy[k] = v
					}
					copy.CrossPostStatus = cpsCopy
				}
				result = append(result, &copy)
				break
			}
		}
	}
	return result, nil
}
