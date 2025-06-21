package conf

import (
	"time"

	"go.orx.me/apps/hyper-sync/internal/social"
)

var (
	Conf *Config
)

type Config struct {
	Socials   map[string]*social.PlatformConfig
	Database  *DatabaseConfig
	Memos     *MemosConfig
	Sync      *SyncConfig
	Scheduler *SchedulerConfig
	Webhook   *WebhookConfig
}

// ServerConfig contains server configuration
type ServerConfig struct {
	HTTPPort string
	GRPCPort string
	LogLevel string
}

// DatabaseConfig contains database configuration
type DatabaseConfig struct {
	MongoURI     string
	DatabaseName string
}

// SyncConfig contains sync-related configuration
type SyncConfig struct {
	Interval        time.Duration
	MaxRetries      int
	BatchSize       int
	MaxMemosPerRun  int
	TargetPlatforms []string
	SkipPrivate     bool
	SkipOlder       time.Duration
}

// SchedulerConfig contains scheduler configuration
type SchedulerConfig struct {
	AutoSyncEnabled    bool
	DefaultInterval    time.Duration
	MaxConcurrentTasks int
	MaxRetries         int
	RetryDelay         time.Duration
	QueueSize          int
	TaskTimeout        time.Duration
	SchedulePatterns   []SchedulePattern
}

// SchedulePattern defines a custom sync schedule
type SchedulePattern struct {
	Name      string
	CronExpr  string
	Enabled   bool
	Platforms []string
	Filters   *SyncFilters
}

// SyncFilters defines filtering criteria for scheduled syncs
type SyncFilters struct {
	MemosCreator string
	SkipPrivate  bool
	SkipOlder    time.Duration
	MaxMemos     int
}

// WebhookConfig contains webhook configuration
type WebhookConfig struct {
	Enabled        bool
	Secret         string
	AllowedSources []string
	TrustedIPs     []string
	Timeout        time.Duration
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
