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
	Config map[string]interface{} `bson:"config"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}

// GetThreadsConfig extracts Threads-specific configuration
func (m *SocialConfigModel) GetThreadsConfig() *social.ThreadsConfig {
	if val, ok := m.Config["threads"]; ok {
		if threadsMap, ok := val.(map[string]interface{}); ok {
			config := &social.ThreadsConfig{}
			if clientID, ok := threadsMap["client_id"].(string); ok {
				config.ClientID = clientID
			}
			if clientSecret, ok := threadsMap["client_secret"].(string); ok {
				config.ClientSecret = clientSecret
			}
			if accessToken, ok := threadsMap["access_token"].(string); ok {
				config.AccessToken = accessToken
			}

			if expiresAt, ok := threadsMap["expires_at"].(time.Time); ok {
				config.ExpiresAt = &expiresAt
			}
			return config
		}
	}
	return nil
}

// SocialConfigDao defines the interface for social platform configuration data access
type SocialConfigDao interface {
	GetConfigByPlatform(ctx context.Context, platform string) (*SocialConfigModel, error)
	UpdatePlatformToken(ctx context.Context, platform, accessToken string, expiresAt *time.Time) error
}

// Ensure MongoDAO implements SocialConfigDao
var _ SocialConfigDao = (*MongoDAO)(nil)

// NewSocialConfigDao creates a new SocialConfigDao

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
func (d *MongoDAO) UpdatePlatformToken(ctx context.Context, platform, accessToken string, expiresAt *time.Time) error {
	collection := d.Client.Database(d.Database).Collection(socialConfigCollection)

	filter := bson.M{"platform": platform}
	update := bson.M{
		"$set": bson.M{
			"config." + platform + ".access_token": accessToken,
			"updated_at":                           time.Now(),
		},
	}

	if expiresAt != nil {
		update["$set"].(bson.M)["config."+platform+".expires_at"] = *expiresAt
	}

	_, err := collection.UpdateOne(ctx, filter, update)
	return err
}
