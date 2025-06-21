package social

// PlatformConfig 包含社交平台的基础配置
type PlatformConfig struct {
	Name              string          // 平台名称
	Type              string          // 平台类型 (mastodon, bluesky)
	Main              bool            // 是否为主平台
	Enabled           bool            // 是否启用
	SyncEnabled       bool            // 是否启用同步功能
	SyncFromPlatforms []string        // 允许从哪些平台同步内容
	SyncCategories    []string        // 要同步的内容类别
	Mastodon          *MastodonConfig // Mastodon 特定配置
	Bluesky           *BlueskyConfig  // Bluesky 特定配置
}

// MastodonConfig 包含 Mastodon 平台的特定配置
type MastodonConfig struct {
	Instance string // Mastodon 实例域名
	Token    string // 访问令牌
}

// BlueskyConfig 包含 Bluesky 平台的特定配置
type BlueskyConfig struct {
	Host     string // Bluesky 服务器
	Handle   string // 用户名
	Password string // 密码
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
