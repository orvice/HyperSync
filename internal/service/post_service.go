package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"butterfly.orx.me/core/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/social"
)

// PostService handles business logic for posts
type PostService struct {
	dao           *dao.MongoDAO
	socialService *SocialService
	jobRunning    bool
	jobMutex      sync.Mutex
}

// NewPostService creates a new post service
func NewPostService(dao *dao.MongoDAO, socialService *SocialService) *PostService {
	return &PostService{
		dao:           dao,
		socialService: socialService,
		jobRunning:    false,
	}
}

// GetPost retrieves a post by ID
func (s *PostService) GetPost(ctx context.Context, id string) (*social.Post, error) {
	// Get the post from the database
	post, err := s.dao.GetPostByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get post: %w", err)
	}

	if post == nil {
		return nil, nil // Not found
	}

	// Convert to social.Post
	return post.ToSocialPost(), nil
}

// ListPosts retrieves posts with optional filtering
func (s *PostService) ListPosts(ctx context.Context, platformFilter string, limit, page int) ([]*social.Post, error) {
	// Create filter
	filter := bson.M{}
	if platformFilter != "" {
		filter["source_platform"] = platformFilter
	}

	// Calculate skip based on page and limit
	var skip int64 = 0
	if page > 1 {
		skip = int64((page - 1) * limit)
	}

	// Get posts from database
	posts, err := s.dao.ListPosts(ctx, filter, int64(limit), skip)
	if err != nil {
		return nil, fmt.Errorf("failed to list posts: %w", err)
	}

	// Convert to social.Post
	result := make([]*social.Post, 0, len(posts))
	for _, post := range posts {
		result = append(result, post.ToSocialPost())
	}

	return result, nil
}

// CreatePost creates a new post and optionally cross-posts
func (s *PostService) CreatePost(ctx context.Context, post *social.Post, platforms []string) (string, error) {
	// Convert to database model
	dbPost := dao.FromSocialPost(post)

	// Create in database
	postID, err := s.dao.CreatePost(ctx, dbPost)
	if err != nil {
		return "", fmt.Errorf("failed to create post: %w", err)
	}

	// If platforms are specified, cross-post
	if len(platforms) > 0 {
		go s.crossPostToTargets(context.Background(), postID, post, platforms)
	}

	return postID, nil
}

// DeletePost deletes a post
func (s *PostService) DeletePost(ctx context.Context, id string) error {
	// Get the post first to check if it exists
	post, err := s.dao.GetPostByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get post for deletion: %w", err)
	}

	if post == nil {
		return fmt.Errorf("post not found")
	}

	// Delete from database
	if err := s.dao.DeletePost(ctx, id); err != nil {
		return fmt.Errorf("failed to delete post: %w", err)
	}

	return nil
}

// crossPostToTargets cross-posts to specified target platforms
func (s *PostService) crossPostToTargets(ctx context.Context, postID string, post *social.Post, platforms []string) {
	logger := log.FromContext(ctx)

	for _, platform := range platforms {
		// Cross-post to the platform
		resp, err := s.socialService.PostToPlatform(ctx, platform, post)

		// Update cross-post status
		now := time.Now()
		status := dao.CrossPostStatus{
			CrossPosted: err == nil,
			PostedAt:    &now,
		}

		if err != nil {
			status.Success = false
			status.Error = err.Error()
			logger.Error("Failed to cross-post", "platform", platform, "error", err)
		} else {
			status.Success = true
			if resp != nil {
				// Try to extract platform ID
				switch v := resp.(type) {
				case map[string]interface{}:
					if id, ok := v["id"].(string); ok {
						status.PlatformID = id
					}
				}
			}
			logger.Info("Cross-post succeeded", "platform", platform, "post_id", postID)
		}

		// Update status in database (ignore errors)
		if err := s.dao.UpdateCrossPostStatus(ctx, postID, platform, status); err != nil {
			logger.Error("Failed to update cross-post status", "platform", platform, "post_id", postID, "error", err)
		}
	}
}

// SyncPost syncs a post from one platform to others
func (s *PostService) SyncPost(ctx context.Context, platformID, postID string, targetPlatforms []string) error {
	// Get the original post from the platform
	post, err := s.socialService.GetPostFromPlatform(ctx, platformID, postID)
	if err != nil {
		return fmt.Errorf("failed to get post from %s: %w", platformID, err)
	}

	// Check if we've already synced this post
	existingPost, err := s.dao.GetPostByOriginalID(ctx, platformID, postID)
	if err != nil {
		return fmt.Errorf("failed to check for existing post: %w", err)
	}

	// Set source information
	post.SourcePlatform = platformID
	post.OriginalID = postID

	if existingPost != nil {
		// This post has already been synced before
		return fmt.Errorf("post already synced with ID: %s", existingPost.ID.Hex())
	}

	// Save to database and cross-post
	_, err = s.CreatePost(ctx, post, targetPlatforms)
	if err != nil {
		return fmt.Errorf("failed to create and sync post: %w", err)
	}

	return nil
}

// StartSyncJob starts a background job that synchronizes posts from all platforms
// according to the configuration
func (s *PostService) StartSyncJob(ctx context.Context, interval time.Duration) error {
	logger := log.FromContext(ctx)

	s.jobMutex.Lock()
	if s.jobRunning {
		s.jobMutex.Unlock()
		return fmt.Errorf("sync job is already running")
	}
	s.jobRunning = true
	s.jobMutex.Unlock()

	go func() {
		jobCtx := context.Background()
		jobLogger := log.FromContext(jobCtx)
		jobLogger.Info("Starting post sync job")

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer func() {
			s.jobMutex.Lock()
			s.jobRunning = false
			s.jobMutex.Unlock()
			jobLogger.Info("Post sync job stopped")
		}()

		// Run immediately on start
		s.runSyncJob(jobCtx)

		for {
			select {
			case <-ticker.C:
				s.runSyncJob(jobCtx)
			case <-ctx.Done():
				return
			}
		}
	}()

	logger.Info("Post sync job started", "interval", interval)
	return nil
}

// StopSyncJob stops the sync job
func (s *PostService) StopSyncJob() {
	s.jobMutex.Lock()
	defer s.jobMutex.Unlock()
	s.jobRunning = false
}

// runSyncJob runs one iteration of the sync job
func (s *PostService) runSyncJob(ctx context.Context) {
	logger := log.FromContext(ctx)
	logger.Info("Running post sync job iteration")

	// Get all configured platforms
	platforms := s.socialService.GetAllPlatforms()

	// For each source platform
	for sourceName, sourcePlatform := range platforms {
		// Skip if not enabled
		if !sourcePlatform.Config.Enabled {
			continue
		}

		// Get posts from this platform
		logger.Info("Fetching posts", "platform", sourceName)
		posts, err := s.socialService.ListPlatformPosts(ctx, sourceName, 20)
		if err != nil {
			logger.Error("Failed to fetch posts", "platform", sourceName, "error", err)
			continue
		}

		// For each post
		for _, post := range posts {
			// Set source information
			post.SourcePlatform = sourceName

			// Check if we've already synced this post
			existingPost, err := s.dao.GetPostByOriginalID(ctx, sourceName, post.ID)
			if err != nil {
				logger.Error("Failed to check for existing post",
					"platform", sourceName,
					"post_id", post.ID,
					"error", err)
				continue
			}

			if existingPost != nil {
				// This post has already been synced
				continue
			}

			// Get target platforms that should receive this post
			var targetPlatforms []string
			for targetName, targetPlatform := range platforms {
				// Skip self
				if targetName == sourceName {
					continue
				}

				// Check if this target should receive posts from the source
				if targetPlatform.Config.ShouldSyncPost(sourceName) {
					targetPlatforms = append(targetPlatforms, targetName)
				}
			}

			// If there are target platforms, sync the post
			if len(targetPlatforms) > 0 {
				logger.Info("Syncing post",
					"post_id", post.ID,
					"source", sourceName,
					"targets", targetPlatforms)

				post.OriginalID = post.ID // Store the original ID
				_, err := s.CreatePost(ctx, post, targetPlatforms)
				if err != nil {
					logger.Error("Failed to sync post",
						"post_id", post.ID,
						"source", sourceName,
						"error", err)
				}
			}
		}
	}

	logger.Info("Post sync job iteration completed")
}
