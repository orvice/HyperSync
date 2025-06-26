package dao

import (
	"context"
	"time"

	"go.orx.me/apps/hyper-sync/internal/social"
)

// ThreadsConfigAdapter 适配器，将 SocialConfigDao 适配为 social.TokenManager
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

	// 直接从 SocialConfig 结构体中获取 token 信息
	var accessToken string
	var expiresAt *time.Time

	// 优先使用 AccessToken 字段
	if configModel.Config.AccessToken != "" {
		accessToken = configModel.Config.AccessToken
		expiresAt = configModel.Config.ExpiresAt
	} else if configModel.Config.Token != "" {
		// 兼容其他平台使用 "token" 字段的情况
		accessToken = configModel.Config.Token
	}

	if accessToken == "" {
		return nil, nil // No token found
	}

	return &social.TokenInfo{
		AccessToken: accessToken,
		ExpiresAt:   expiresAt,
	}, nil
}

// SaveAccessToken 保存指定平台的 access token
func (a *ThreadsConfigAdapter) SaveAccessToken(ctx context.Context, platform, accessToken string, expiresAt *time.Time) error {
	// 直接使用 SocialConfigDao 的 UpdatePlatformToken 方法
	return a.socialDao.UpdatePlatformToken(ctx, platform, accessToken, expiresAt)
}

// Ensure ThreadsConfigAdapter implements social.TokenManager
var _ social.TokenManager = (*ThreadsConfigAdapter)(nil)
