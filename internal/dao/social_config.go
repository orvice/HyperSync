package dao

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"go.orx.me/apps/hyper-sync/internal/social"
)

const (
	socialConfigCollection = "social_configs"
)

// SocialConfigDao defines the interface for social config data access operations
type SocialConfigDao interface {
	// GetConfigByPlatform retrieves social config by platform name
	GetConfigByPlatform(ctx context.Context, platform string) (*SocialConfigModel, error)

	// GetAllConfigs retrieves all social platform configs
	GetAllConfigs(ctx context.Context) ([]*SocialConfigModel, error)

	// CreateConfig creates a new social config
	CreateConfig(ctx context.Context, config *SocialConfigModel) (string, error)

	// UpdateConfig updates an existing social config
	UpdateConfig(ctx context.Context, config *SocialConfigModel) error

	// UpdatePlatformToken updates the access token for a specific platform
	UpdatePlatformToken(ctx context.Context, platform, accessToken string, expiresAt *time.Time) error

	// DeleteConfig deletes a social config by platform name
	DeleteConfig(ctx context.Context, platform string) error
}

// SocialConfigModel represents a social platform configuration in the database
type SocialConfigModel struct {
	ID        bson.ObjectID          `bson:"_id,omitempty"`
	Platform  string                 `bson:"platform"` // 平台名称，如 "threads", "mastodon", "bluesky"
	Type      string                 `bson:"type"`     // 平台类型
	Enabled   bool                   `bson:"enabled"`  // 是否启用
	Config    map[string]interface{} `bson:"config"`   // 平台特定配置
	CreatedAt time.Time              `bson:"created_at"`
	UpdatedAt time.Time              `bson:"updated_at"`
}

// ThreadsConfigData represents the Threads specific configuration data
type ThreadsConfigData struct {
	ClientID     string     `bson:"client_id" json:"client_id"`
	ClientSecret string     `bson:"client_secret" json:"client_secret"`
	AccessToken  string     `bson:"access_token" json:"access_token"`
	TokenType    string     `bson:"token_type,omitempty" json:"token_type,omitempty"`
	ExpiresAt    *time.Time `bson:"expires_at,omitempty" json:"expires_at,omitempty"`
}

// FromPlatformConfig converts a social.PlatformConfig to SocialConfigModel
func FromPlatformConfig(config *social.PlatformConfig) *SocialConfigModel {
	now := time.Now()
	configData := make(map[string]interface{})

	// Convert platform specific configs to map
	if config.Mastodon != nil {
		configData["mastodon"] = map[string]interface{}{
			"instance": config.Mastodon.Instance,
			"token":    config.Mastodon.Token,
		}
	}
	if config.Bluesky != nil {
		configData["bluesky"] = map[string]interface{}{
			"host":     config.Bluesky.Host,
			"handle":   config.Bluesky.Handle,
			"password": config.Bluesky.Password,
		}
	}
	if config.Memos != nil {
		configData["memos"] = map[string]interface{}{
			"endpoint": config.Memos.Endpoint,
			"token":    config.Memos.Token,
		}
	}

	return &SocialConfigModel{
		Platform:  config.Name,
		Type:      config.Type,
		Enabled:   config.Enabled,
		Config:    configData,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// ToPlatformConfig converts SocialConfigModel to social.PlatformConfig
func (s *SocialConfigModel) ToPlatformConfig() *social.PlatformConfig {
	config := &social.PlatformConfig{
		Name:    s.Platform,
		Type:    s.Type,
		Enabled: s.Enabled,
	}

	// Convert map back to platform specific configs
	if mastodonData, ok := s.Config["mastodon"].(map[string]interface{}); ok {
		config.Mastodon = &social.MastodonConfig{
			Instance: getString(mastodonData, "instance"),
			Token:    getString(mastodonData, "token"),
		}
	}
	if blueskyData, ok := s.Config["bluesky"].(map[string]interface{}); ok {
		config.Bluesky = &social.BlueskyConfig{
			Host:     getString(blueskyData, "host"),
			Handle:   getString(blueskyData, "handle"),
			Password: getString(blueskyData, "password"),
		}
	}
	if memosData, ok := s.Config["memos"].(map[string]interface{}); ok {
		config.Memos = &social.MemosConfig{
			Endpoint: getString(memosData, "endpoint"),
			Token:    getString(memosData, "token"),
		}
	}

	return config
}

// GetThreadsConfig extracts Threads specific configuration
func (s *SocialConfigModel) GetThreadsConfig() *ThreadsConfigData {
	if threadsData, ok := s.Config["threads"].(map[string]interface{}); ok {
		config := &ThreadsConfigData{
			ClientID:     getString(threadsData, "client_id"),
			ClientSecret: getString(threadsData, "client_secret"),
			AccessToken:  getString(threadsData, "access_token"),
			TokenType:    getString(threadsData, "token_type"),
		}
		if expiresAt, ok := threadsData["expires_at"].(time.Time); ok {
			config.ExpiresAt = &expiresAt
		}
		return config
	}
	return nil
}

// SetThreadsConfig sets Threads specific configuration
func (s *SocialConfigModel) SetThreadsConfig(config *ThreadsConfigData) {
	if s.Config == nil {
		s.Config = make(map[string]interface{})
	}
	threadsData := map[string]interface{}{
		"client_id":     config.ClientID,
		"client_secret": config.ClientSecret,
		"access_token":  config.AccessToken,
	}
	if config.TokenType != "" {
		threadsData["token_type"] = config.TokenType
	}
	if config.ExpiresAt != nil {
		threadsData["expires_at"] = *config.ExpiresAt
	}
	s.Config["threads"] = threadsData
	s.UpdatedAt = time.Now()
}

// Helper function to safely get string from map
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// Ensure MongoDAO implements SocialConfigDao interface
var _ SocialConfigDao = (*MongoDAO)(nil)

// GetConfigByPlatform retrieves social config by platform name
func (d *MongoDAO) GetConfigByPlatform(ctx context.Context, platform string) (*SocialConfigModel, error) {
	collection := d.Client.Database(d.Database).Collection(socialConfigCollection)

	config := &SocialConfigModel{}
	err := collection.FindOne(ctx, bson.M{"platform": platform}).Decode(config)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // Not found
		}
		return nil, err
	}

	return config, nil
}

// GetAllConfigs retrieves all social platform configs
func (d *MongoDAO) GetAllConfigs(ctx context.Context) ([]*SocialConfigModel, error) {
	collection := d.Client.Database(d.Database).Collection(socialConfigCollection)

	opts := options.Find().SetSort(bson.D{{Key: "platform", Value: 1}})
	cursor, err := collection.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var configs []*SocialConfigModel
	if err := cursor.All(ctx, &configs); err != nil {
		return nil, err
	}

	return configs, nil
}

// CreateConfig creates a new social config
func (d *MongoDAO) CreateConfig(ctx context.Context, config *SocialConfigModel) (string, error) {
	collection := d.Client.Database(d.Database).Collection(socialConfigCollection)

	// Ensure timestamps are set
	now := time.Now()
	config.CreatedAt = now
	config.UpdatedAt = now

	result, err := collection.InsertOne(ctx, config)
	if err != nil {
		return "", err
	}

	// Return the inserted ID as string
	if objectID, ok := result.InsertedID.(bson.ObjectID); ok {
		return objectID.Hex(), nil
	}

	return "", nil
}

// UpdateConfig updates an existing social config
func (d *MongoDAO) UpdateConfig(ctx context.Context, config *SocialConfigModel) error {
	collection := d.Client.Database(d.Database).Collection(socialConfigCollection)

	// Update timestamp
	config.UpdatedAt = time.Now()

	filter := bson.M{"platform": config.Platform}
	update := bson.M{"$set": config}

	_, err := collection.UpdateOne(ctx, filter, update)
	return err
}

// UpdatePlatformToken updates the access token for a specific platform
func (d *MongoDAO) UpdatePlatformToken(ctx context.Context, platform, accessToken string, expiresAt *time.Time) error {
	collection := d.Client.Database(d.Database).Collection(socialConfigCollection)

	update := bson.M{
		"$set": bson.M{
			"config." + platform + ".access_token": accessToken,
			"updated_at":                           time.Now(),
		},
	}

	if expiresAt != nil {
		update["$set"].(bson.M)["config."+platform+".expires_at"] = *expiresAt
	}

	filter := bson.M{"platform": platform}
	_, err := collection.UpdateOne(ctx, filter, update)
	return err
}

// DeleteConfig deletes a social config by platform name
func (d *MongoDAO) DeleteConfig(ctx context.Context, platform string) error {
	collection := d.Client.Database(d.Database).Collection(socialConfigCollection)

	_, err := collection.DeleteOne(ctx, bson.M{"platform": platform})
	return err
}
