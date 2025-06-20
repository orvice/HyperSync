package app

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"butterfly.orx.me/core/log"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.orx.me/apps/hyper-sync/internal/conf"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/service"
)

// ApiServer represents the API server with all components
type ApiServer struct {
	// Core services
	mongoDAO      *dao.MongoDAO
	socialService *service.SocialService
	postService   *service.PostService
	syncService   *service.SyncService

	// Configuration
	config *AppConfig

	// State
	isInitialized bool
}

// AppConfig contains the application configuration
type AppConfig struct {
	// Database configuration
	MongoURI     string
	DatabaseName string

	// Sync configuration
	SyncConfig *service.SyncConfig

	// Server configuration
	HTTPPort string
	GRPCPort string

	// Logging
	LogLevel string
}

// NewApiServer creates a new API server instance
func NewApiServer() (*ApiServer, error) {
	config, err := loadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	return &ApiServer{
		config:        config,
		isInitialized: false,
	}, nil
}

// Initialize initializes all components of the API server
func (s *ApiServer) Initialize(ctx context.Context, mongoClient *mongo.Client) error {
	logger := log.FromContext(ctx)
	logger.Info("Initializing API server components")

	// Initialize MongoDB DAO
	s.mongoDAO = dao.NewMongoDAO(mongoClient, s.config.DatabaseName)

	// Initialize social service
	socialService, err := service.NewSocialService(conf.Conf.Socials)
	if err != nil {
		return fmt.Errorf("failed to initialize social service: %w", err)
	}
	s.socialService = socialService

	// Initialize post service
	s.postService = service.NewPostService(s.mongoDAO, s.socialService)

	// Initialize sync service if configured
	if s.config.SyncConfig != nil {
		syncService, err := service.NewSyncService(s.mongoDAO, s.socialService, s.config.SyncConfig)
		if err != nil {
			logger.Warn("Failed to initialize sync service", "error", err)
		} else {
			s.syncService = syncService
			logger.Info("Sync service initialized successfully")
		}
	}

	s.isInitialized = true
	logger.Info("API server components initialized successfully")

	return nil
}

// GetMongoDAO returns the MongoDB DAO instance
func (s *ApiServer) GetMongoDAO() *dao.MongoDAO {
	return s.mongoDAO
}

// GetSocialService returns the social service instance
func (s *ApiServer) GetSocialService() *service.SocialService {
	return s.socialService
}

// GetPostService returns the post service instance
func (s *ApiServer) GetPostService() *service.PostService {
	return s.postService
}

// GetSyncService returns the sync service instance
func (s *ApiServer) GetSyncService() *service.SyncService {
	return s.syncService
}

// IsInitialized returns whether the server is initialized
func (s *ApiServer) IsInitialized() bool {
	return s.isInitialized
}

// GetConfig returns the application configuration
func (s *ApiServer) GetConfig() *AppConfig {
	return s.config
}

// StartSyncJob starts the background sync job if sync service is available
func (s *ApiServer) StartSyncJob(ctx context.Context) error {
	if s.syncService == nil {
		return fmt.Errorf("sync service not initialized")
	}

	logger := log.FromContext(ctx)
	logger.Info("Starting background sync job",
		"interval", s.config.SyncConfig.SyncInterval,
		"target_platforms", s.config.SyncConfig.TargetPlatforms,
	)

	// For now, we don't start the automatic job as it was in the original post service
	// This would need to be implemented based on the requirements
	logger.Info("Background sync job setup completed")
	return nil
}

// TriggerManualSync triggers a manual sync operation
func (s *ApiServer) TriggerManualSync(ctx context.Context) (*service.SyncResult, error) {
	if s.syncService == nil {
		return nil, fmt.Errorf("sync service not initialized")
	}

	logger := log.FromContext(ctx)
	logger.Info("Triggering manual sync")

	result, err := s.syncService.SyncFromMemos(ctx)
	if err != nil {
		logger.Error("Manual sync failed", "error", err)
		return result, err
	}

	logger.Info("Manual sync completed successfully",
		"total_checked", result.TotalMemosChecked,
		"synced", result.MemosSynced,
		"skipped", result.MemosSkipped,
	)

	return result, nil
}

// GetSyncStatus returns the current sync status
func (s *ApiServer) GetSyncStatus(ctx context.Context) (map[string]interface{}, error) {
	if s.syncService == nil {
		return map[string]interface{}{
			"status": "sync_service_not_initialized",
		}, nil
	}

	return s.syncService.GetSyncStatus(ctx)
}

// Shutdown gracefully shuts down the API server
func (s *ApiServer) Shutdown(ctx context.Context) error {
	logger := log.FromContext(ctx)
	logger.Info("Shutting down API server")

	// TODO: Stop any running background jobs
	// TODO: Close database connections if needed
	// TODO: Clean up resources

	logger.Info("API server shutdown completed")
	return nil
}

// loadConfig loads configuration from environment variables and config files
func loadConfig() (*AppConfig, error) {
	config := &AppConfig{
		// Default values
		MongoURI:     getEnv("MONGO_URI", "mongodb://localhost:27017"),
		DatabaseName: getEnv("DATABASE_NAME", "hyper_sync"),
		HTTPPort:     getEnv("HTTP_PORT", "8080"),
		GRPCPort:     getEnv("GRPC_PORT", "9090"),
		LogLevel:     getEnv("LOG_LEVEL", "info"),
	}

	// Load sync configuration
	syncConfig, err := loadSyncConfig()
	if err != nil {
		// Don't fail if sync config is not available
		// We can't use log here since it requires a context
		// This will be logged when the service is actually initialized
	} else {
		config.SyncConfig = syncConfig
	}

	return config, nil
}

// loadSyncConfig loads sync-specific configuration
func loadSyncConfig() (*service.SyncConfig, error) {
	memosEndpoint := getEnv("MEMOS_ENDPOINT", "")
	memosToken := getEnv("MEMOS_TOKEN", "")

	if memosEndpoint == "" {
		return nil, fmt.Errorf("MEMOS_ENDPOINT not configured")
	}

	// Parse interval
	intervalStr := getEnv("SYNC_INTERVAL", "15m")
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		interval = 15 * time.Minute
	}

	// Parse skip older duration
	skipOlderStr := getEnv("SYNC_SKIP_OLDER", "168h") // 7 days
	skipOlder, err := time.ParseDuration(skipOlderStr)
	if err != nil {
		skipOlder = 7 * 24 * time.Hour
	}

	return &service.SyncConfig{
		MaxRetries:      getEnvInt("SYNC_MAX_RETRIES", 3),
		SyncInterval:    interval,
		BatchSize:       getEnvInt("SYNC_BATCH_SIZE", 20),
		MaxMemosPerRun:  getEnvInt("SYNC_MAX_MEMOS_PER_RUN", 100),
		TargetPlatforms: getEnvSlice("SYNC_TARGET_PLATFORMS", []string{"mastodon", "bluesky"}),
		MemosConfig: &service.MemosConfig{
			Endpoint: memosEndpoint,
			Token:    memosToken,
			Creator:  getEnv("MEMOS_CREATOR", ""),
		},
		SkipPrivate: getEnvBool("SYNC_SKIP_PRIVATE", true),
		SkipOlder:   skipOlder,
	}, nil
}

// Helper functions for environment variable handling
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1"
	}
	return defaultValue
}

func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		// Simple comma-separated parsing
		parts := strings.Split(value, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}
