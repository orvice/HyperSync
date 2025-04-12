package service

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/social"
)

// PostService handles business logic for posts
type PostService struct {
	dao           *dao.MongoDAO
	socialService *SocialService
}

// NewPostService creates a new post service
func NewPostService(dao *dao.MongoDAO, socialService *SocialService) *PostService {
	return &PostService{
		dao:           dao,
		socialService: socialService,
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
		}

		// Update status in database (ignore errors)
		_ = s.dao.UpdateCrossPostStatus(ctx, postID, platform, status)
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
