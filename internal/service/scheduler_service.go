package service

import (
	"context"
	"fmt"
	"time"

	"butterfly.orx.me/core/log"
	"github.com/bsm/redislock"
	"go.orx.me/apps/hyper-sync/internal/social"
)

// SchedulerService handles scheduled tasks like token refresh
type SchedulerService struct {
	socialService *SocialService
	locker        *redislock.Client
	tokenManager  social.TokenManager
}

// NewSchedulerService creates a new scheduler service
func NewSchedulerService(socialService *SocialService, locker *redislock.Client, tokenManager social.TokenManager) *SchedulerService {
	return &SchedulerService{
		socialService: socialService,
		locker:        locker,
		tokenManager:  tokenManager,
	}
}

// StartTokenRefreshScheduler 启动 token 刷新定时任务
// 每隔指定时间检查所有平台的 token 是否需要刷新
func (s *SchedulerService) StartTokenRefreshScheduler(ctx context.Context, interval time.Duration) {
	logger := log.FromContext(ctx)

	logger.Info("Starting token refresh scheduler", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 立即执行一次检查
	s.RefreshAllTokens(ctx)

	for {
		select {
		case <-ctx.Done():
			logger.Info("Token refresh scheduler stopped")
			return
		case <-ticker.C:
			s.RefreshAllTokens(ctx)
		}
	}
}

// RefreshAllTokens 检查并刷新所有平台的 token
func (s *SchedulerService) RefreshAllTokens(ctx context.Context) {
	logger := log.FromContext(ctx).With("method", "RefreshAllTokens")

	// 使用分布式锁确保同一时间只有一个实例在执行 token 刷新
	lock, err := s.locker.Obtain(ctx, "token_refresh", 5*time.Minute, nil)
	if err != nil {
		logger.Warn("Failed to obtain token refresh lock, skipping", "error", err)
		return
	}
	defer lock.Release(ctx)

	logger.Info("Starting token refresh for all platforms")

	platforms := s.socialService.GetAllPlatforms()

	for platformName, platform := range platforms {
		logger.Info("Checking token for platform",
			"platform", platformName)

		err := s.RefreshPlatformToken(ctx, platformName, platform)
		if err != nil {
			logger.Error("Failed to refresh token for platform",
				"platform", platformName,
				"error", err)
		}
	}

	logger.Info("Completed token refresh check for all platforms")
}

// RefreshPlatformToken 刷新指定平台的 token
func (s *SchedulerService) RefreshPlatformToken(ctx context.Context, platformName string, platform *social.SocialPlatform) error {
	logger := log.FromContext(ctx)

	// 目前只处理 Threads 平台
	if platform.Config.Type != "threads" {
		logger.Debug("Skipping non-threads platform", "platform", platformName, "type", platform.Config.Type)
		return nil
	}

	// 类型断言获取 ThreadsClient
	threadsClient, ok := platform.Client.(*social.ThreadsClient)
	if !ok {
		logger.Error("Platform client is not a ThreadsClient", "platform", platformName)
		return fmt.Errorf("platform %s client is not a ThreadsClient", platformName)
	}

	logger.Info("Checking token for Threads platform", "platform", platformName)

	// 检查并刷新 token（如果需要）
	err := threadsClient.EnsureValidToken(ctx)
	if err != nil {
		logger.Error("Failed to ensure valid token",
			"platform", platformName,
			"error", err)
		return fmt.Errorf("failed to ensure valid token for platform %s: %w", platformName, err)
	}

	logger.Info("Token check completed successfully", "platform", platformName)
	return nil
}

// RefreshThreadsTokenManually 手动刷新 Threads token（用于测试或紧急情况）
func (s *SchedulerService) RefreshThreadsTokenManually(ctx context.Context, platformName string) error {
	logger := log.FromContext(ctx).With("method", "RefreshThreadsTokenManually")
	ctx = log.WithLogger(ctx, logger)

	platform, err := s.socialService.GetPlatform(platformName)
	if err != nil {
		return fmt.Errorf("platform not found: %w", err)
	}

	if platform.Config.Type != "threads" {
		return fmt.Errorf("platform %s is not a threads platform", platformName)
	}

	logger.Info("Getting platform", "platform", platformName)
	threadsClient, ok := platform.Client.(*social.ThreadsClient)
	if !ok {
		return fmt.Errorf("platform %s client is not a ThreadsClient", platformName)
	}

	logger.Info("Manually refreshing token for Threads platform", "platform", platformName)

	// 强制刷新 token
	tokenResp, err := threadsClient.RefreshLongLivedToken()
	if err != nil {
		logger.Error("Failed to refresh token manually",
			"platform", platformName,
			"error", err)
		return fmt.Errorf("failed to refresh token manually for platform %s: %w", platformName, err)
	}

	// 保存新的 token 到数据库
	err = threadsClient.SaveTokenToDao(ctx, tokenResp)
	if err != nil {
		logger.Error("Failed to save refreshed token",
			"platform", platformName,
			"error", err)
		return fmt.Errorf("failed to save refreshed token for platform %s: %w", platformName, err)
	}

	logger.Info("Token manually refreshed successfully",
		"platform", platformName,
		"new_expiry", tokenResp.GetTokenExpirationTime().Format(time.RFC3339))

	return nil
}

// GetTokenStatus 获取指定平台的 token 状态信息
func (s *SchedulerService) GetTokenStatus(ctx context.Context, platformName string) (*TokenStatus, error) {
	platform, err := s.socialService.GetPlatform(platformName)
	if err != nil {
		return nil, fmt.Errorf("platform not found: %w", err)
	}

	if platform.Config.Type != "threads" {
		return &TokenStatus{
			PlatformName: platformName,
			PlatformType: platform.Config.Type,
			HasToken:     false,
			Message:      "Token management not supported for this platform type",
		}, nil
	}

	// 获取 token 信息
	tokenInfo, err := s.tokenManager.GetTokenInfo(ctx, platformName)
	if err != nil {
		return nil, fmt.Errorf("failed to get token info: %w", err)
	}

	status := &TokenStatus{
		PlatformName: platformName,
		PlatformType: platform.Config.Type,
	}

	if tokenInfo == nil || tokenInfo.AccessToken == "" {
		status.HasToken = false
		status.Message = "No token found"
		return status, nil
	}

	status.HasToken = true
	status.ExpiresAt = tokenInfo.ExpiresAt

	if tokenInfo.ExpiresAt == nil {
		status.Message = "Token found but no expiration time"
		status.IsExpiringSoon = false
	} else {
		timeUntilExpiry := time.Until(*tokenInfo.ExpiresAt)
		status.TimeUntilExpiry = &timeUntilExpiry

		// 检查是否在 7 天内过期
		const refreshThreshold = 7 * 24 * time.Hour
		status.IsExpiringSoon = timeUntilExpiry <= refreshThreshold

		if timeUntilExpiry <= 0 {
			status.Message = "Token has expired"
		} else if status.IsExpiringSoon {
			status.Message = fmt.Sprintf("Token expires in %s, will be refreshed soon", timeUntilExpiry.Round(time.Hour))
		} else {
			status.Message = fmt.Sprintf("Token is valid, expires in %s", timeUntilExpiry.Round(time.Hour))
		}
	}

	return status, nil
}

// TokenStatus 表示 token 的状态信息
type TokenStatus struct {
	PlatformName    string         `json:"platform_name"`
	PlatformType    string         `json:"platform_type"`
	HasToken        bool           `json:"has_token"`
	ExpiresAt       *time.Time     `json:"expires_at,omitempty"`
	TimeUntilExpiry *time.Duration `json:"time_until_expiry,omitempty"`
	IsExpiringSoon  bool           `json:"is_expiring_soon"`
	Message         string         `json:"message"`
}
