package conf

import (
	"time"

	"go.orx.me/apps/hyper-sync/internal/social"
)

var (
	Conf *Config
)

type Config struct {
	Socials map[string]*social.PlatformConfig
	Auth    *AuthConfig // Add Auth config section
}

func (c *Config) Print() {}

type SocialConfig struct {
	// Social type
	Type              string
	Enabled           bool
	SyncEnabled       bool     // Whether to sync content to this platform from others
	SyncFromPlatforms []string // List of platform IDs to sync content from
	SyncCategories    []string // Categories of content to sync (e.g., "posts", "images", "replies")

	// Mastodon config
	Mastodon *MastodonConfig
	// Bluesky config
	Bluesky *BlueskyConfig
}

type MastodonConfig struct {
	Instance string
	Token    string
}

type BlueskyConfig struct {
	Host     string
	Handle   string
	Password string
}

// AuthConfig holds authentication related configurations
type AuthConfig struct {
	Google *GoogleConfig
	JWT    *JWTConfig
}

// GoogleConfig holds Google OAuth credentials
type GoogleConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// JWTConfig holds JWT secret and expiration settings
type JWTConfig struct {
	SecretKey  string
	Expiration time.Duration
}
