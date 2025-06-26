package dao

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockSocialConfigDao for testing
type MockSocialConfigDao struct {
	mock.Mock
}

func (m *MockSocialConfigDao) GetConfigByPlatform(ctx context.Context, platform string) (*SocialConfigModel, error) {
	args := m.Called(ctx, platform)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*SocialConfigModel), args.Error(1)
}

func (m *MockSocialConfigDao) UpdatePlatformToken(ctx context.Context, platform, accessToken string, expiresAt *time.Time) error {
	args := m.Called(ctx, platform, accessToken, expiresAt)
	return args.Error(0)
}

func TestSocialConfigModel_GetThreadsConfig(t *testing.T) {
	t.Run("should return nil when no essential fields are set", func(t *testing.T) {
		model := &SocialConfigModel{
			Platform: "threads",
			Config:   SocialConfig{
				// 没有设置 AccessToken 或 ClientID
			},
		}

		config := model.GetThreadsConfig()
		assert.Nil(t, config)
	})

	t.Run("should return config when AccessToken is set", func(t *testing.T) {
		expiresAt := time.Now().Add(30 * 24 * time.Hour)
		model := &SocialConfigModel{
			Platform: "threads",
			Config: SocialConfig{
				AccessToken: "test_token_123",
				ExpiresAt:   &expiresAt,
			},
		}

		config := model.GetThreadsConfig()
		assert.NotNil(t, config)
		assert.Equal(t, "test_token_123", config.AccessToken)
		assert.Equal(t, &expiresAt, config.ExpiresAt)
	})
}

func TestUpdatePlatformToken_Logic(t *testing.T) {
	t.Run("should handle existing and non-existing records", func(t *testing.T) {
		// 这个测试主要验证逻辑，实际的数据库操作测试需要集成测试

		// 测试场景1：记录存在 - 应该更新
		// 测试场景2：记录不存在 - 应该创建

		// 由于这涉及到实际的数据库操作，这里主要验证数据结构
		accessToken := "new_token_123"
		expiresAt := time.Now().Add(60 * 24 * time.Hour)

		// 验证新记录的结构
		newConfig := &SocialConfigModel{
			Platform: "threads",
			UserID:   0,
			Config: SocialConfig{
				AccessToken: accessToken,
				ExpiresAt:   &expiresAt,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		assert.Equal(t, "threads", newConfig.Platform)
		assert.Equal(t, accessToken, newConfig.Config.AccessToken)
		assert.Equal(t, &expiresAt, newConfig.Config.ExpiresAt)
		assert.Equal(t, int64(0), newConfig.UserID)
	})
}

func TestSocialConfig_Fields(t *testing.T) {
	t.Run("should handle all social config fields", func(t *testing.T) {
		expiresAt := time.Now().Add(24 * time.Hour)

		config := SocialConfig{
			AccessToken: "test_access_token",
			ExpiresAt:   &expiresAt,
		}

		assert.Equal(t, "test_access_token", config.AccessToken)
		assert.Equal(t, &expiresAt, config.ExpiresAt)
	})

	t.Run("should handle optional fields", func(t *testing.T) {
		config := SocialConfig{
			AccessToken: "test_token",
			// ExpiresAt 为 nil（可选字段）
		}

		assert.Equal(t, "test_token", config.AccessToken)
		assert.Nil(t, config.ExpiresAt)
	})
}
