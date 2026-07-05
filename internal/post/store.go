package post

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("post not found")

type Post struct {
	ID              string
	Content         string
	Visibility      string
	Status          string
	MediaIDs        []string
	SyncTargets     []string
	CrossPostStatus map[string]CrossPostStatus
	// SyncPending marks posts the publish worker still has work for.
	// Set by the service on create/publish/update, recomputed by the worker.
	SyncPending bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CrossPostStatus struct {
	Success     bool
	Error       string
	PlatformID  string
	PostedAt    *time.Time
	RetryCount  int
	NeedsUpdate bool
}

type ListOptions struct {
	PageSize int
	Page     int
	Status   string
}

type ListResult struct {
	Posts []*Post
	Total int
}

type Store interface {
	Create(ctx context.Context, post *Post) (*Post, error)
	GetByID(ctx context.Context, id string) (*Post, error)
	List(ctx context.Context, opts ListOptions) (*ListResult, error)
	Update(ctx context.Context, post *Post) (*Post, error)
	Delete(ctx context.Context, id string) error
	ListPendingSync(ctx context.Context) ([]*Post, error)
	// UpdateSyncStatus atomically updates a single platform's cross-post status
	// without touching the rest of the document, so the worker cannot clobber
	// concurrent user edits.
	UpdateSyncStatus(ctx context.Context, id, platform string, status CrossPostStatus) error
	// SetSyncPending flips the worker's pending flag for a post.
	SetSyncPending(ctx context.Context, id string, pending bool) error
}

// ComputeSyncPending reports whether the publish worker still has work to do
// for this post given the retry budget.
func ComputeSyncPending(p *Post, maxRetries int) bool {
	if p.Status != "published" || len(p.SyncTargets) == 0 {
		return false
	}
	for _, target := range p.SyncTargets {
		status, ok := p.CrossPostStatus[target]
		if !ok {
			return true
		}
		if status.RetryCount >= maxRetries {
			continue
		}
		if !status.Success || status.NeedsUpdate {
			return true
		}
	}
	return false
}
