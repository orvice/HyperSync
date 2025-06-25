package dao

import (
	"context"
	"time"

	"go.orx.me/apps/hyper-sync/internal/social"
)

// ThreadsConfigAdapter 适配器，将 SocialConfigDao 适配为 social.ConfigDao
// 简化版本，只处理 access token
type ThreadsConfigAdapter struct {
	socialDao SocialConfigDao
}

// NewThreadsConfigAdapter 创建新的 Threads 配置适配器
func NewThreadsConfigAdapter(socialDao SocialConfigDao) *ThreadsConfigAdapter {
	return &ThreadsConfigAdapter{
		socialDao: socialDao,
	}
}

// GetAccessToken 获取指定平台的 access token
func (a *ThreadsConfigAdapter) GetAccessToken(ctx context.Context, platform string) (string, error) {
	tokenInfo, err := a.GetTokenInfo(ctx, platform)
	if err != nil {
		return "", err
	}
	if tokenInfo == nil {
		return "", nil
	}
	return tokenInfo.AccessToken, nil
}

// GetTokenInfo 获取指定平台的 token 信息（包含过期时间）
func (a *ThreadsConfigAdapter) GetTokenInfo(ctx context.Context, platform string) (*social.TokenInfo, error) {
	configModel, err := a.socialDao.GetConfigByPlatform(ctx, platform)
	if err != nil {
		return nil, err
	}

	if configModel == nil {
		return nil, nil // Not found
	}

	// 根据平台类型获取 token 信息
	switch platform {
	case "threads":
		threadsConfig := configModel.GetThreadsConfig()
		if threadsConfig != nil {
			return &social.TokenInfo{
				AccessToken: threadsConfig.AccessToken,
				ExpiresAt:   threadsConfig.ExpiresAt,
			}, nil
		}
	case "mastodon":
		// 从通用配置中获取 mastodon token（mastodon token 通常不会过期）
		if mastodonData, ok := configModel.Config["mastodon"].(map[string]interface{}); ok {
			if token, ok := mastodonData["token"].(string); ok {
				return &social.TokenInfo{
					AccessToken: token,
					ExpiresAt:   nil, // Mastodon token 通常不过期
				}, nil
			}
		}
	case "bluesky":
		// Bluesky 可能不使用传统的 access token，这里预留接口
		if blueskyData, ok := configModel.Config["bluesky"].(map[string]interface{}); ok {
			if token, ok := blueskyData["access_token"].(string); ok {
				return &social.TokenInfo{
					AccessToken: token,
					ExpiresAt:   nil, // 需要根据 Bluesky 的实际情况调整
				}, nil
			}
		}
	case "memos":
		// 从通用配置中获取 memos token
		if memosData, ok := configModel.Config["memos"].(map[string]interface{}); ok {
			if token, ok := memosData["token"].(string); ok {
				return &social.TokenInfo{
					AccessToken: token,
					ExpiresAt:   nil, // Memos token 通常不过期
				}, nil
			}
		}
	}

	return nil, nil // No token found for this platform
}

// SaveAccessToken 保存指定平台的 access token
func (a *ThreadsConfigAdapter) SaveAccessToken(ctx context.Context, platform, accessToken string, expiresAt *time.Time) error {
	// 直接使用 SocialConfigDao 的 UpdatePlatformToken 方法
	return a.socialDao.UpdatePlatformToken(ctx, platform, accessToken, expiresAt)
}

// Ensure ThreadsConfigAdapter implements social.ConfigDao
var _ social.ConfigDao = (*ThreadsConfigAdapter)(nil)
