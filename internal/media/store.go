package media

import (
	"context"
	"errors"
	"io"
	"time"
)

var ErrNotFound = errors.New("media not found")

type Media struct {
	ID               string
	S3Key            string
	CDNUrl           string
	ContentType      string
	SizeBytes        int64
	OriginalFilename string
	CreatedAt        time.Time
}

type ListOptions struct {
	PageSize int
	Page     int
}

type ListResult struct {
	Items []*Media
	Total int
}

type Store interface {
	Create(ctx context.Context, m *Media) (*Media, error)
	GetByID(ctx context.Context, id string) (*Media, error)
	List(ctx context.Context, opts ListOptions) (*ListResult, error)
	Delete(ctx context.Context, id string) error
}

type ObjectStorage interface {
	Upload(ctx context.Context, key string, contentType string, body io.Reader) error
	Delete(ctx context.Context, key string) error
}
