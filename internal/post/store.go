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
	CreatedAt       time.Time
	UpdatedAt       time.Time
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
}
