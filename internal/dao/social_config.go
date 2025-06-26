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
	ClientID     string     `bson:"client_id,omitempty"`
	ClientSecret string     `bson:"client_secret,omitempty"`
	UserID       int64      `bson:"user_id,omitempty"`
	ExpiresAt    *time.Time `bson:"expires_at,omitempty"`

	// Other platform fields can be added here as needed
	Token    string `bson:"token,omitempty"`    // For platforms using "token" instead of "access_token"
	Instance string `bson:"instance,omitempty"` // For Mastodon
	Handle   string `bson:"handle,omitempty"`   // For Bluesky
	Password string `bson:"password,omitempty"` // For Bluesky
	Endpoint string `bson:"endpoint,omitempty"` // For Memos
}

// GetThreadsConfig extracts Threads-specific configuration
func (m *SocialConfigModel) GetThreadsConfig() *social.ThreadsConfig {
	config := &social.ThreadsConfig{
		ClientID:     m.Config.ClientID,
		ClientSecret: m.Config.ClientSecret,
		AccessToken:  m.Config.AccessToken,
		UserID:       m.Config.UserID,
		ExpiresAt:    m.Config.ExpiresAt,
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
func (d *MongoDAO) UpdatePlatformToken(ctx context.Context, platform, accessToken string, expiresAt *time.Time) error {
	collection := d.Client.Database(d.Database).Collection(socialConfigCollection)

	filter := bson.M{"platform": platform}
	update := bson.M{
		"$set": bson.M{
			"config.access_token": accessToken,
			"updated_at":          time.Now(),
		},
	}

	if expiresAt != nil {
		update["$set"].(bson.M)["config.expires_at"] = *expiresAt
	}

	_, err := collection.UpdateOne(ctx, filter, update)
	return err
}
