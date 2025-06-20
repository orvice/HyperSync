package conf

import "go.orx.me/apps/hyper-sync/internal/social"

var (
	Conf *Config
)

type Config struct {
	Socials map[string]*social.PlatformConfig
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

	// Memos config
	Memos *MemosConfig
}

type MemosConfig struct {
	Endpoint string
	Token    string
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
