package post

import (
	"context"
	"fmt"
	"sort"
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

// clonePost deep-copies a post so callers never share the stored map/slices.
func clonePost(p *Post) *Post {
	c := *p
	if p.MediaIDs != nil {
		c.MediaIDs = append([]string(nil), p.MediaIDs...)
	}
	if p.SyncTargets != nil {
		c.SyncTargets = append([]string(nil), p.SyncTargets...)
	}
	if p.CrossPostStatus != nil {
		c.CrossPostStatus = make(map[string]CrossPostStatus, len(p.CrossPostStatus))
		for k, v := range p.CrossPostStatus {
			c.CrossPostStatus[k] = v
		}
	}
	return &c
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

	s.posts[p.ID] = clonePost(p)
	return clonePost(p), nil
}

func (s *MemoryStore) GetByID(_ context.Context, id string) (*Post, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.posts[id]
	if !ok {
		return nil, ErrNotFound
	}
	return clonePost(p), nil
}

func (s *MemoryStore) List(_ context.Context, opts ListOptions) (*ListResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var filtered []*Post
	for _, p := range s.posts {
		if opts.Status != "" && p.Status != opts.Status {
			continue
		}
		filtered = append(filtered, clonePost(p))
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].CreatedAt.Equal(filtered[j].CreatedAt) {
			return filtered[i].ID > filtered[j].ID
		}
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})

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
	s.posts[p.ID] = clonePost(p)
	return clonePost(p), nil
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

func (s *MemoryStore) UpdateSyncStatus(_ context.Context, id, platform string, status CrossPostStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.posts[id]
	if !ok {
		return ErrNotFound
	}
	if p.CrossPostStatus == nil {
		p.CrossPostStatus = make(map[string]CrossPostStatus)
	}
	p.CrossPostStatus[platform] = status
	return nil
}

func (s *MemoryStore) SetSyncPending(_ context.Context, id string, pending bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.posts[id]
	if !ok {
		return ErrNotFound
	}
	p.SyncPending = pending
	return nil
}

func (s *MemoryStore) ListPendingSync(_ context.Context) ([]*Post, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Post
	for _, p := range s.posts {
		if p.Status != "published" || !p.SyncPending {
			continue
		}
		if len(p.SyncTargets) == 0 {
			continue
		}
		result = append(result, clonePost(p))
	}
	return result, nil
}
