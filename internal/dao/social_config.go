package dao

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.orx.me/apps/hyper-sync/internal/social"
)

const (
	socialConfigCollection = "social_configs"
)

// SocialConfigModel represents the social platform configuration in the database
type SocialConfigModel struct {
	ID       bson.ObjectID `bson:"_id,omitempty"`
	Platform string        `bson:"platform"`
	UserID   int64         `bson:"user_id"` // Optional: for multi-user support

	// Config stores platform-specific settings
	Config SocialConfig `bson:"config"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}

type SocialConfig struct {
	AccessToken string `bson:"access_token,omitempty"`

	// Threads specific fields
	ExpiresAt *time.Time `bson:"expires_at,omitempty"`
}

// GetThreadsConfig extracts Threads-specific configuration
func (m *SocialConfigModel) GetThreadsConfig() *social.ThreadsConfig {
	config := &social.ThreadsConfig{
		AccessToken: m.Config.AccessToken,
		ExpiresAt:   m.Config.ExpiresAt,
	}

	// Return nil if no essential fields are set
	if config.AccessToken == "" && config.ClientID == "" {
		return nil
	}

	return config
}

// SocialConfigDao defines the interface for social platform configuration data access
type SocialConfigDao interface {
	GetConfigByPlatform(ctx context.Context, platform string) (*SocialConfigModel, error)
	UpdatePlatformToken(ctx context.Context, platform, accessToken string, expiresAt *time.Time) error
}

// Ensure MongoDAO implements SocialConfigDao
var _ SocialConfigDao = (*MongoDAO)(nil)

// GetConfigByPlatform retrieves a social platform's configuration
func (d *MongoDAO) GetConfigByPlatform(ctx context.Context, platform string) (*SocialConfigModel, error) {
	collection := d.Client.Database(d.Database).Collection(socialConfigCollection)

	var config SocialConfigModel
	err := collection.FindOne(ctx, bson.M{"platform": platform}).Decode(&config)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // Not found is not an error
		}
		return nil, err
	}
	return &config, nil
}

// UpdatePlatformToken updates the access token for a specific platform
// If the platform config doesn't exist, it creates a new one
func (d *MongoDAO) UpdatePlatformToken(ctx context.Context, platform, accessToken string, expiresAt *time.Time) error {
	collection := d.Client.Database(d.Database).Collection(socialConfigCollection)

	// 先尝试获取现有记录
	existingConfig, err := d.GetConfigByPlatform(ctx, platform)
	if err != nil {
		return err
	}

	now := time.Now()

	if existingConfig != nil {
		// 记录存在，执行更新
		filter := bson.M{"platform": platform}
		update := bson.M{
			"$set": bson.M{
				"config.access_token": accessToken,
				"updated_at":          now,
			},
		}

		if expiresAt != nil {
			update["$set"].(bson.M)["config.expires_at"] = *expiresAt
		}

		_, err := collection.UpdateOne(ctx, filter, update)
		return err
	} else {
		// 记录不存在，创建新记录
		newConfig := &SocialConfigModel{
			Platform: platform,
			UserID:   0, // 默认为0，可以根据需要修改
			Config: SocialConfig{
				AccessToken: accessToken,
				ExpiresAt:   expiresAt,
			},
			CreatedAt: now,
			UpdatedAt: now,
		}

		_, err := collection.InsertOne(ctx, newConfig)
		return err
	}
}
