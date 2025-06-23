package social

// PlatformConfig 包含社交平台的基础配置
type PlatformConfig struct {
	Name string `yaml:"name"` // 平台名称
	Type string `yaml:"type"` // 平台类型 (mastodon, bluesky, memos, twitter)
	// Main              bool            `yaml:"main"`                // 是否为主平台
	Enabled           bool     `yaml:"enabled"`             // 是否启用
	SyncEnabled       bool     `yaml:"sync_enabled"`        // 是否启用同步功能
	SyncTo            []string `yaml:"sync_to"`             // 同步到哪些平台
	SyncFromPlatforms []string `yaml:"sync_from_platforms"` // 允许从哪些平台同步内容
	SyncCategories    []string `yaml:"sync_categories"`     // 要同步的内容类别

	Mastodon *MastodonConfig `yaml:"mastodon,omitempty"` // Mastodon 特定配置
	Bluesky  *BlueskyConfig  `yaml:"bluesky,omitempty"`  // Bluesky 特定配置
	Memos    *MemosConfig    `yaml:"memos,omitempty"`    // Memos 特定配置
	Twitter  *TwitterConfig  `yaml:"twitter,omitempty"`  // Twitter 特定配置
}

type MemosConfig struct {
	Endpoint string `yaml:"endpoint"`
	Token    string `yaml:"token"`
}

// MastodonConfig 包含 Mastodon 平台的特定配置
type MastodonConfig struct {
	Instance string `yaml:"instance"` // Mastodon 实例域名
	Token    string `yaml:"token"`    // 访问令牌
}

// BlueskyConfig 包含 Bluesky 平台的特定配置
type BlueskyConfig struct {
	Host     string `yaml:"host"`     // Bluesky 服务器
	Handle   string `yaml:"handle"`   // 用户名
	Password string `yaml:"password"` // 密码
}

type TwitterConfig struct {
	ConsumerKey    string `yaml:"consumer_key"`
	ConsumerSecret string `yaml:"consumer_secret"`
	AccessToken    string `yaml:"access_token"`
	AccessSecret   string `yaml:"access_secret"`
}

// ShouldSyncPost 判断是否应该将内容从源平台同步到目标平台
func (c *PlatformConfig) ShouldSyncPost(sourcePlatform string) bool {
	// 如果同步功能未启用，不同步
	if !c.SyncEnabled {
		return false
	}

	// 检查源平台是否在允许同步的平台列表中
	for _, platform := range c.SyncFromPlatforms {
		if platform == sourcePlatform || platform == "*" {
			return true
		}
	}

	return false
}
