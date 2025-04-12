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
	ID             string
	Content        string
	Visibility     string
	Media          []Media
	SourcePlatform string
	OriginalID     string
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
	Config map[string]interface{}
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
		if !ShouldSyncPost(sourcePlatform, platform.Config) {
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
func InitSocialPlatforms(configs map[string]map[string]interface{}) ([]*SocialPlatform, error) {
	var platforms []*SocialPlatform

	for name, config := range configs {
		// Check if this platform is enabled
		enabled, ok := config["Enabled"].(bool)
		if !ok || !enabled {
			continue
		}

		// Get platform type
		platformType, ok := config["Type"].(string)
		if !ok {
			return nil, fmt.Errorf("missing platform type for %s", name)
		}

		var client SocialClient
		var err error

		// Initialize the appropriate client based on type
		switch platformType {
		case "mastodon":
			mastodonConfig, ok := config["Mastodon"].(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("missing Mastodon config for %s", name)
			}

			instance, _ := mastodonConfig["Instance"].(string)
			token, _ := mastodonConfig["Token"].(string)

			if instance == "" || token == "" {
				return nil, fmt.Errorf("missing Mastodon credentials for %s", name)
			}

			client = NewMastodonClient(instance, token)

		case "bluesky":
			blueskyConfig, ok := config["Bluesky"].(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("missing Bluesky config for %s", name)
			}

			host, _ := blueskyConfig["Host"].(string)
			handle, _ := blueskyConfig["Handle"].(string)
			password, _ := blueskyConfig["Password"].(string)

			if host == "" || handle == "" || password == "" {
				return nil, fmt.Errorf("missing Bluesky credentials for %s", name)
			}

			client, err = NewBlueskyClient(host, handle, password)
			if err != nil {
				return nil, fmt.Errorf("failed to initialize Bluesky client for %s: %w", name, err)
			}

		default:
			return nil, fmt.Errorf("unsupported platform type %s for %s", platformType, name)
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
