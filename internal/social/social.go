package social

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type SocialClient interface {
	Post(ctx context.Context, post *Post) (interface{}, error)
	ListPosts(ctx context.Context, limit int) ([]*Post, error)
}

type Post struct {
	ID         string
	Content    string
	Visibility string
	Media      []Media
}

type Media struct {
	data        []byte
	url         string
	Description string
}

// NewMedia creates a new Media object from byte data
func NewMedia(data []byte) *Media {
	return &Media{data: data}
}

// NewMediaFromURL creates a new Media object from a URL
func NewMediaFromURL(url string) *Media {
	return &Media{url: url}
}

// GetData returns the media data, fetching from URL if necessary
func (m *Media) GetData() ([]byte, error) {
	// If we already have the data, return it
	if m.data != nil {
		return m.data, nil
	}

	// If we have a URL, fetch the data
	if m.url != "" {
		// Create HTTP client with timeout
		client := &http.Client{
			Timeout: 30 * time.Second,
		}

		// Make the request
		resp, err := client.Get(m.url)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch media from URL %s: %w", m.url, err)
		}
		defer resp.Body.Close()

		// Check status code
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to fetch media from URL %s: status code %d", m.url, resp.StatusCode)
		}

		// Read the body
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read media data from URL %s: %w", m.url, err)
		}

		// Cache the data for future calls
		m.data = data
		return data, nil
	}

	// No data and no URL
	return nil, fmt.Errorf("media has no data and no URL")
}
