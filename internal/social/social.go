package social

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TokenInfo contains token and expiration information
type TokenInfo struct {
	AccessToken string
	ExpiresAt   *time.Time
}

// TokenManager defines the interface for managing access tokens
type TokenManager interface {
	GetAccessToken(ctx context.Context, platform string) (string, error)
	GetTokenInfo(ctx context.Context, platform string) (*TokenInfo, error)
	SaveAccessToken(ctx context.Context, platform, accessToken string, expiresAt *time.Time) error
}

type SocialClient interface {
	Post(ctx context.Context, post *Post) (interface{}, error)
	ListPosts(ctx context.Context, limit int) ([]*Post, error)
	Name() string
}

type Post struct {
	ID             string
	Content        string
	Visibility     string
	Media          []Media
	SourcePlatform string
	OriginalID     string

	CreatedAt time.Time
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

// GetURL returns the media URL if available
func (m *Media) GetURL() string {
	return m.url
}

// ShouldSyncPost determines if a post should be synced from source to target platform
// based on the provided configuration
func ShouldSyncPost(sourcePlatform string, targetPlatformConfig map[string]interface{}) bool {
	// Check if sync is enabled at all
	syncEnabled, ok := targetPlatformConfig["SyncEnabled"].(bool)
	if !ok || !syncEnabled {
		return false
	}

	// Check if the source platform is in the list of platforms to sync from
	syncFromPlatforms, ok := targetPlatformConfig["SyncFromPlatforms"].([]string)
	if !ok || len(syncFromPlatforms) == 0 {
		return false
	}

	// Check if this specific source platform is allowed
	for _, platform := range syncFromPlatforms {
		if platform == sourcePlatform || platform == "*" {
			return true
		}
	}

	return false
}

// SocialPlatform represents a platform configuration with its client
type SocialPlatform struct {
	Name   string
	Client SocialClient
	Config *PlatformConfig
}

// CrossPost posts content to multiple social platforms based on configuration
func CrossPost(ctx context.Context, post *Post, platforms []*SocialPlatform) (map[string]interface{}, error) {
	results := make(map[string]interface{})
	var firstError error

	// Remember the source platform
	sourcePlatform := post.SourcePlatform

	// Post to each enabled platform that should receive this content
	for _, platform := range platforms {
		// Skip the source platform (don't repost to where it came from)
		if platform.Name == sourcePlatform {
			continue
		}

		// Check if this post should be synced to this platform
		if !platform.Config.ShouldSyncPost(sourcePlatform) {
			continue
		}

		// Post to this platform
		resp, err := platform.Client.Post(ctx, post)

		// Store the result
		if err != nil {
			results[platform.Name] = map[string]interface{}{
				"success": false,
				"error":   err.Error(),
			}

			// Remember the first error
			if firstError == nil {
				firstError = fmt.Errorf("failed to post to %s: %w", platform.Name, err)
			}
		} else {
			results[platform.Name] = map[string]interface{}{
				"success":  true,
				"response": resp,
			}
		}
	}

	return results, firstError
}

// InitSocialPlatforms initializes social clients from configuration
func InitSocialPlatforms(configs map[string]*PlatformConfig, tokenManager TokenManager) ([]*SocialPlatform, error) {
	var platforms []*SocialPlatform

	for name, config := range configs {
		// Skip disabled platforms
		if !config.Enabled {
			continue
		}

		// Set name if not already set
		if config.Name == "" {
			config.Name = name
		}

		var client SocialClient
		var err error

		// Initialize the appropriate client based on type
		switch config.Type {
		case "memos":
			if config.Memos == nil {
				return nil, fmt.Errorf("missing Memos config for %s", name)
			}
			if config.Memos.Endpoint == "" || config.Memos.Token == "" {
				return nil, fmt.Errorf("missing Memos credentials for %s", name)
			}
			client = NewMemos(config.Memos.Endpoint, config.Memos.Token, config.Name)

		case "mastodon":
			if config.Mastodon == nil {
				return nil, fmt.Errorf("missing Mastodon config for %s", name)
			}

			if config.Mastodon.Instance == "" || config.Mastodon.Token == "" {
				return nil, fmt.Errorf("missing Mastodon credentials for %s", name)
			}

			client = NewMastodonClient(config.Mastodon.Instance, config.Mastodon.Token, config.Name)

		case "bluesky":
			if config.Bluesky == nil {
				return nil, fmt.Errorf("missing Bluesky config for %s", name)
			}

			if config.Bluesky.Host == "" || config.Bluesky.Handle == "" || config.Bluesky.Password == "" {
				return nil, fmt.Errorf("missing Bluesky credentials for %s", name)
			}

			client, err = NewBlueskyClient(config.Bluesky.Host, config.Bluesky.Handle, config.Bluesky.Password, config.Name)
			if err != nil {
				return nil, fmt.Errorf("failed to initialize Bluesky client for %s: %w", name, err)
			}

		case "threads":
			if config.Threads == nil {
				return nil, fmt.Errorf("missing Threads config for %s", name)
			}
			client, err = NewThreadsClientWithDao(config.Name, config.Threads.ClientID, config.Threads.ClientSecret, config.Threads.AccessToken,
				config.Threads.UserID, tokenManager)
			if err != nil {
				return nil, fmt.Errorf("failed to initialize Threads client for %s: %w", name, err)
			}

		default:
			return nil, fmt.Errorf("unsupported platform type %s for %s", config.Type, name)
		}

		// Add the platform to the list
		platforms = append(platforms, &SocialPlatform{
			Name:   name,
			Client: client,
			Config: config,
		})
	}

	return platforms, nil
}
