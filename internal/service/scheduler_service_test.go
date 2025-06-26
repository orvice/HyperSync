package service

import (
	"context"
	"testing"
	"time"

	"github.com/bsm/redislock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.orx.me/apps/hyper-sync/internal/social"
)

// MockTokenManager 模拟 TokenManager
type MockTokenManager struct {
	mock.Mock
}

func (m *MockTokenManager) GetAccessToken(ctx context.Context, platform string) (string, error) {
	args := m.Called(ctx, platform)
	return args.String(0), args.Error(1)
}

func (m *MockTokenManager) GetTokenInfo(ctx context.Context, platform string) (*social.TokenInfo, error) {
	args := m.Called(ctx, platform)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*social.TokenInfo), args.Error(1)
}

func (m *MockTokenManager) SaveAccessToken(ctx context.Context, platform, accessToken string, expiresAt *time.Time) error {
	args := m.Called(ctx, platform, accessToken, expiresAt)
	return args.Error(0)
}

// MockRedisClient 模拟 Redis 客户端
type MockRedisClient struct {
	mock.Mock
}

// MockRedisLock 模拟 Redis 锁
type MockRedisLock struct {
	mock.Mock
}

func (m *MockRedisLock) Release(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockRedisClient) Obtain(ctx context.Context, key string, ttl time.Duration, opt *redislock.Options) (*redislock.Lock, error) {
	args := m.Called(ctx, key, ttl, opt)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*redislock.Lock), args.Error(1)
}

func TestSchedulerService_GetTokenStatus(t *testing.T) {
	// 创建模拟的 TokenManager
	mockTokenManager := &MockTokenManager{}
	mockRedisClient := &redislock.Client{}

	// 创建模拟的 SocialService
	socialService := &SocialService{
		platforms: map[string]*social.SocialPlatform{
			"threads": {
				Name: "threads",
				Config: &social.PlatformConfig{
					Type: "threads",
				},
			},
			"mastodon": {
				Name: "mastodon",
				Config: &social.PlatformConfig{
					Type: "mastodon",
				},
			},
		},
	}

	schedulerService := NewSchedulerService(socialService, mockRedisClient, mockTokenManager)

	t.Run("should return token status for threads platform", func(t *testing.T) {
		ctx := context.Background()
		expiresAt := time.Now().Add(30 * 24 * time.Hour) // 30 days from now

		mockTokenManager.On("GetTokenInfo", ctx, "threads").Return(&social.TokenInfo{
			AccessToken: "test_token_123",
			ExpiresAt:   &expiresAt,
		}, nil)

		status, err := schedulerService.GetTokenStatus(ctx, "threads")

		assert.NoError(t, err)
		assert.NotNil(t, status)
		assert.Equal(t, "threads", status.PlatformName)
		assert.Equal(t, "threads", status.PlatformType)
		assert.True(t, status.HasToken)
		assert.False(t, status.IsExpiringSoon)
		assert.NotNil(t, status.ExpiresAt)

		mockTokenManager.AssertExpectations(t)
	})

	t.Run("should return token status for non-threads platform", func(t *testing.T) {
		ctx := context.Background()

		status, err := schedulerService.GetTokenStatus(ctx, "mastodon")

		assert.NoError(t, err)
		assert.NotNil(t, status)
		assert.Equal(t, "mastodon", status.PlatformName)
		assert.Equal(t, "mastodon", status.PlatformType)
		assert.False(t, status.HasToken)
		assert.Contains(t, status.Message, "not supported")
	})

	t.Run("should return error for unknown platform", func(t *testing.T) {
		ctx := context.Background()

		status, err := schedulerService.GetTokenStatus(ctx, "unknown")

		assert.Error(t, err)
		assert.Nil(t, status)
		assert.Contains(t, err.Error(), "platform not found")
	})

	t.Run("should handle token expiring soon", func(t *testing.T) {
		ctx := context.Background()
		expiresAt := time.Now().Add(3 * 24 * time.Hour) // 3 days from now (within 7 day threshold)

		mockTokenManager.On("GetTokenInfo", ctx, "threads").Return(&social.TokenInfo{
			AccessToken: "test_token_123",
			ExpiresAt:   &expiresAt,
		}, nil)

		status, err := schedulerService.GetTokenStatus(ctx, "threads")

		assert.NoError(t, err)
		assert.NotNil(t, status)
		assert.True(t, status.HasToken)
		assert.True(t, status.IsExpiringSoon)
		assert.Contains(t, status.Message, "expires in")

		mockTokenManager.AssertExpectations(t)
	})

	t.Run("should handle expired token", func(t *testing.T) {
		ctx := context.Background()
		expiresAt := time.Now().Add(-1 * time.Hour) // 1 hour ago (expired)

		mockTokenManager.On("GetTokenInfo", ctx, "threads").Return(&social.TokenInfo{
			AccessToken: "test_token_123",
			ExpiresAt:   &expiresAt,
		}, nil)

		status, err := schedulerService.GetTokenStatus(ctx, "threads")

		assert.NoError(t, err)
		assert.NotNil(t, status)
		assert.True(t, status.HasToken)
		assert.True(t, status.IsExpiringSoon)
		assert.Contains(t, status.Message, "expired")

		mockTokenManager.AssertExpectations(t)
	})

	t.Run("should handle missing token", func(t *testing.T) {
		ctx := context.Background()

		mockTokenManager.On("GetTokenInfo", ctx, "threads").Return(nil, nil)

		status, err := schedulerService.GetTokenStatus(ctx, "threads")

		assert.NoError(t, err)
		assert.NotNil(t, status)
		assert.False(t, status.HasToken)
		assert.Contains(t, status.Message, "No token found")

		mockTokenManager.AssertExpectations(t)
	})
}
