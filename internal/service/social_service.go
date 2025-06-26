package service

import (
	"context"
	"fmt"

	"go.orx.me/apps/hyper-sync/internal/conf"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/social"
)

// SocialService handles interactions with social platforms
type SocialService struct {
	platforms map[string]*social.SocialPlatform
}

// NewSocialService creates a new social service
func NewSocialService(threadsDao *dao.ThreadsConfigAdapter) (*SocialService, error) {
	config := conf.Conf.Socials
	// Initialize platforms with the configuration
	platforms, err := social.InitSocialPlatforms(config, threadsDao)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize social platforms: %w", err)
	}

	// Create a map for easier access
	platformMap := make(map[string]*social.SocialPlatform)
	for _, platform := range platforms {
		platformMap[platform.Name] = platform
	}

	return &SocialService{
		platforms: platformMap,
	}, nil
}

// GetPlatform gets a platform by name
func (s *SocialService) GetPlatform(name string) (*social.SocialPlatform, error) {
	platform, ok := s.platforms[name]
	if !ok {
		return nil, fmt.Errorf("platform not found: %s", name)
	}
	return platform, nil
}

// GetAllPlatforms returns all configured platforms
func (s *SocialService) GetAllPlatforms() map[string]*social.SocialPlatform {
	return s.platforms
}

// PostToPlatform posts content to a specific platform
func (s *SocialService) PostToPlatform(ctx context.Context, platformName string, post *social.Post) (interface{}, error) {
	platform, err := s.GetPlatform(platformName)
	if err != nil {
		return nil, err
	}

	// Post to the platform
	return platform.Client.Post(ctx, post)
}

// GetPostFromPlatform gets a post from a specific platform
func (s *SocialService) GetPostFromPlatform(ctx context.Context, platformName, postID string) (*social.Post, error) {
	platform, err := s.GetPlatform(platformName)
	if err != nil {
		return nil, err
	}

	// Get posts from the platform
	posts, err := platform.Client.ListPosts(ctx, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to list posts from %s: %w", platformName, err)
	}

	// Find the specific post
	for _, post := range posts {
		if post.ID == postID {
			return post, nil
		}
	}

	return nil, fmt.Errorf("post not found on platform %s: %s", platformName, postID)
}

// ListPlatformPosts lists posts from a specific platform
func (s *SocialService) ListPlatformPosts(ctx context.Context, platformName string, limit int) ([]*social.Post, error) {
	platform, err := s.GetPlatform(platformName)
	if err != nil {
		return nil, err
	}

	// Get posts from the platform
	return platform.Client.ListPosts(ctx, limit)
}
