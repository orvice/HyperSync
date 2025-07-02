package social

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Platform represents different social media platforms
type Platform string

// Platform constants
const (
	PlatformMastodon Platform = "mastodon"
	PlatformBluesky  Platform = "bluesky"
	PlatformThreads  Platform = "threads"
	PlatformMemos    Platform = "memos"
	PlatformTelegram Platform = "telegram"
)

// String returns the string representation of the platform
func (p Platform) String() string {
	return string(p)
}

// IsValid checks if the platform is a valid one
func (p Platform) IsValid() bool {
	switch p {
	case PlatformMastodon, PlatformBluesky, PlatformThreads, PlatformMemos, PlatformTelegram:
		return true
	default:
		return false
	}
}

// ParsePlatform converts a string to Platform enum
func ParsePlatform(platformStr string) Platform {
	return Platform(platformStr)
}

// VisibilityLevel represents the visibility level of a post using type-safe enum
type VisibilityLevel int

const (
	VisibilityLevelPublic   VisibilityLevel = iota // Public posts visible to everyone
	VisibilityLevelUnlisted                        // Public but not shown in public timelines
	VisibilityLevelPrivate                         // Only visible to followers/approved users
	VisibilityLevelDirect                          // Direct messages (Mastodon only)
)

// String returns the string representation of the visibility level
func (v VisibilityLevel) String() string {
	switch v {
	case VisibilityLevelPublic:
		return "public"
	case VisibilityLevelUnlisted:
		return "unlisted"
	case VisibilityLevelPrivate:
		return "private"
	case VisibilityLevelDirect:
		return "direct"
	default:
		return "unknown"
	}
}

// IsValid checks if the visibility level is valid
func (v VisibilityLevel) IsValid() bool {
	return v >= VisibilityLevelPublic && v <= VisibilityLevelDirect
}

// Legacy string constants for backward compatibility
const (
	VisibilityPublic   = "public"
	VisibilityUnlisted = "unlisted"
	VisibilityPrivate  = "private"
	VisibilityDirect   = "direct"

	// Memos platform specific constants
	MemosVisibilityPublic    = "PUBLIC"
	MemosVisibilityProtected = "PROTECTED"
	MemosVisibilityPrivate   = "PRIVATE"
)

// SupportedVisibilityLevels defines which visibility levels are supported by each platform (using enum)
var SupportedVisibilityLevels = map[Platform][]VisibilityLevel{
	PlatformMastodon: {VisibilityLevelPublic, VisibilityLevelUnlisted, VisibilityLevelPrivate, VisibilityLevelDirect},
	PlatformBluesky:  {VisibilityLevelPublic, VisibilityLevelPrivate},
	PlatformThreads:  {VisibilityLevelPublic, VisibilityLevelPrivate},
	PlatformMemos:    {VisibilityLevelPublic, VisibilityLevelUnlisted, VisibilityLevelPrivate},
	PlatformTelegram: {VisibilityLevelPublic, VisibilityLevelPrivate},
}

// DefaultVisibilityLevel defines the default visibility for each platform (using enum)
var DefaultVisibilityLevel = map[Platform]VisibilityLevel{
	PlatformMastodon: VisibilityLevelPublic,
	PlatformBluesky:  VisibilityLevelPublic,
	PlatformThreads:  VisibilityLevelPublic,
	PlatformMemos:    VisibilityLevelPublic,
	PlatformTelegram: VisibilityLevelPublic,
}

// Legacy SupportedVisibilityLevelsString for backward compatibility
var SupportedVisibilityLevelsString = map[string][]string{
	"mastodon": {VisibilityPublic, VisibilityUnlisted, VisibilityPrivate, VisibilityDirect},
	"bluesky":  {VisibilityPublic, VisibilityPrivate},
	"threads":  {VisibilityPublic, VisibilityPrivate},
	"memos":    {VisibilityPublic, VisibilityUnlisted, VisibilityPrivate},
	"telegram": {VisibilityPublic, VisibilityPrivate},
}

// DefaultVisibility defines the default visibility for each platform (string)
var DefaultVisibility = map[string]string{
	"mastodon": VisibilityPublic,
	"bluesky":  VisibilityPublic,
	"threads":  VisibilityPublic,
	"memos":    VisibilityPublic,
	"telegram": VisibilityPublic,
}

// ParseVisibilityLevel converts a string visibility value to VisibilityLevel enum
func ParseVisibilityLevel(visibility string) (VisibilityLevel, error) {
	switch visibility {
	case VisibilityPublic:
		return VisibilityLevelPublic, nil
	case VisibilityUnlisted:
		return VisibilityLevelUnlisted, nil
	case VisibilityPrivate:
		return VisibilityLevelPrivate, nil
	case VisibilityDirect:
		return VisibilityLevelDirect, nil
	default:
		return VisibilityLevel(-1), fmt.Errorf("unknown visibility level: %s", visibility)
	}
}

// ParsePlatformVisibility converts platform-specific visibility string to VisibilityLevel enum
func ParsePlatformVisibility(platform, visibility string) (VisibilityLevel, error) {
	// Handle Memos specific values first
	if platform == PlatformMemos.String() {
		switch visibility {
		case MemosVisibilityPublic:
			return VisibilityLevelPublic, nil
		case MemosVisibilityProtected:
			return VisibilityLevelUnlisted, nil
		case MemosVisibilityPrivate:
			return VisibilityLevelPrivate, nil
		}
	}

	// For other platforms or if Memos value not matched, use standard parsing
	return ParseVisibilityLevel(visibility)
}

// ValidateVisibilityLevel checks if the given visibility level is valid for the specified platform
func ValidateVisibilityLevel(platform string, level VisibilityLevel) error {
	if !level.IsValid() {
		return fmt.Errorf("invalid visibility level: %d", level)
	}

	// Convert string to Platform type
	platformType := ParsePlatform(platform)

	// Get supported levels for the platform
	supportedLevels, exists := SupportedVisibilityLevels[platformType]
	if !exists {
		// If platform is not explicitly defined, allow common visibility values
		supportedLevels = []VisibilityLevel{VisibilityLevelPublic, VisibilityLevelUnlisted, VisibilityLevelPrivate}
	}

	// Check if the visibility is supported
	for _, supportedLevel := range supportedLevels {
		if level == supportedLevel {
			return nil
		}
	}

	// Convert levels to strings for error message
	var supportedStrings []string
	for _, l := range supportedLevels {
		supportedStrings = append(supportedStrings, l.String())
	}

	return fmt.Errorf("visibility '%s' is not supported by platform '%s'. Supported values: %v",
		level.String(), platform, supportedStrings)
}

// IsVisibilityLevelSupported checks if the given visibility level is supported by the platform without returning an error
func IsVisibilityLevelSupported(platform string, level VisibilityLevel) bool {
	if !level.IsValid() {
		return false
	}

	// Convert string to Platform type
	platformType := ParsePlatform(platform)

	// Get supported levels for the platform
	supportedLevels, exists := SupportedVisibilityLevels[platformType]
	if !exists {
		// If platform is not explicitly defined, allow common visibility values
		supportedLevels = []VisibilityLevel{VisibilityLevelPublic, VisibilityLevelUnlisted, VisibilityLevelPrivate}
	}

	// Check if the visibility is supported
	for _, supportedLevel := range supportedLevels {
		if level == supportedLevel {
			return true
		}
	}

	return false
}

// ValidateVisibility checks if the given visibility value is valid for the specified platform (legacy string version)
func ValidateVisibility(platform, visibility string) error {
	if visibility == "" {
		return nil // Empty visibility is allowed, will use platform default
	}

	// Parse the visibility string to enum
	level, err := ParsePlatformVisibility(platform, visibility)
	if err != nil {
		return err
	}

	// Validate using the enum version
	return ValidateVisibilityLevel(platform, level)
}

// GetPlatformVisibilityString converts VisibilityLevel enum to platform-specific string
func GetPlatformVisibilityString(platform string, level VisibilityLevel) string {
	// Handle Memos specific conversion
	if platform == PlatformMemos.String() {
		switch level {
		case VisibilityLevelPublic:
			return MemosVisibilityPublic
		case VisibilityLevelUnlisted:
			return MemosVisibilityProtected
		case VisibilityLevelPrivate:
			return MemosVisibilityPrivate
		default:
			return MemosVisibilityPublic // default fallback
		}
	}

	// For other platforms, return standard string representation
	return level.String()
}

// NormalizeVisibilityLevel converts platform-specific visibility string to VisibilityLevel enum
func NormalizeVisibilityLevel(platform, visibility string) (VisibilityLevel, error) {
	if visibility == "" {
		// Convert string to Platform type
		platformType := ParsePlatform(platform)
		return DefaultVisibilityLevel[platformType], nil
	}

	// Parse the platform-specific visibility to enum
	return ParsePlatformVisibility(platform, visibility)
}

// ValidateAndNormalizeVisibilityLevel validates and normalizes visibility for a given platform (enum version)
func ValidateAndNormalizeVisibilityLevel(platform, visibility string) (VisibilityLevel, error) {
	// First normalize to enum
	level, err := NormalizeVisibilityLevel(platform, visibility)
	if err != nil {
		return VisibilityLevel(-1), err
	}

	// Then validate
	if err := ValidateVisibilityLevel(platform, level); err != nil {
		return VisibilityLevel(-1), err
	}

	return level, nil
}

// Legacy functions for backward compatibility

// NormalizeVisibility converts platform-specific visibility values to common values (legacy string version)
func NormalizeVisibility(platform, visibility string) string {
	if visibility == "" {
		return DefaultVisibility[platform]
	}

	// Handle Memos specific values
	if platform == PlatformMemos.String() {
		switch visibility {
		case MemosVisibilityPublic:
			return VisibilityPublic
		case MemosVisibilityProtected:
			return VisibilityUnlisted
		case MemosVisibilityPrivate:
			return VisibilityPrivate
		}
	}

	// For other platforms, return as-is after validation
	return visibility
}

// GetPlatformVisibility converts common visibility values to platform-specific values (legacy string version)
func GetPlatformVisibility(platform, visibility string) string {
	if visibility == "" {
		return DefaultVisibility[platform]
	}

	// Handle Memos specific conversion
	if platform == PlatformMemos.String() {
		switch visibility {
		case VisibilityPublic:
			return MemosVisibilityPublic
		case VisibilityUnlisted:
			return MemosVisibilityProtected
		case VisibilityPrivate:
			return MemosVisibilityPrivate
		}
	}

	// For other platforms, return as-is
	return visibility
}

// ValidateAndNormalizeVisibility validates and normalizes visibility for a given platform (legacy string version)
func ValidateAndNormalizeVisibility(platform, visibility string) (string, error) {
	normalized := NormalizeVisibility(platform, visibility)

	if err := ValidateVisibility(platform, normalized); err != nil {
		return "", err
	}

	return normalized, nil
}

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
	Visibility     VisibilityLevel
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
		case PlatformMemos.String():
			if config.Memos == nil {
				return nil, fmt.Errorf("missing Memos config for %s", name)
			}
			if config.Memos.Endpoint == "" || config.Memos.Token == "" {
				return nil, fmt.Errorf("missing Memos credentials for %s", name)
			}
			client = NewMemos(config.Memos.Endpoint, config.Memos.Token, config.Name)

		case PlatformMastodon.String():
			if config.Mastodon == nil {
				return nil, fmt.Errorf("missing Mastodon config for %s", name)
			}

			if config.Mastodon.Instance == "" || config.Mastodon.Token == "" {
				return nil, fmt.Errorf("missing Mastodon credentials for %s", name)
			}

			client = NewMastodonClient(config.Mastodon.Instance, config.Mastodon.Token, config.Name)

		case PlatformBluesky.String():
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

		case PlatformThreads.String():
			if config.Threads == nil {
				return nil, fmt.Errorf("missing Threads config for %s", name)
			}
			client, err = NewThreadsClientWithDao(config.Name, config.Threads.ClientID, config.Threads.ClientSecret, config.Threads.AccessToken,
				config.Threads.UserID, tokenManager)
			if err != nil {
				return nil, fmt.Errorf("failed to initialize Threads client for %s: %w", name, err)
			}

		case PlatformTelegram.String():
			if config.Telegram == nil {
				return nil, fmt.Errorf("missing Telegram config for %s", name)
			}
			if config.Telegram.Token == "" || config.Telegram.ChatID == "" {
				return nil, fmt.Errorf("missing Telegram credentials for %s", name)
			}
			client = NewTelegram(config.Telegram.Token, config.Telegram.ChatID, config.Name)

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
