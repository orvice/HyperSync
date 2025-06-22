package service

import (
	"context"
	"time"

	"butterfly.orx.me/core/log"
	"go.orx.me/apps/hyper-sync/internal/conf"
	"go.orx.me/apps/hyper-sync/internal/dao"
)

type SyncService struct {
	socialService *SocialService
	postDao       dao.PostDao

	mainSocail string
	socials    []string
}

func NewSyncService(dao dao.PostDao) (*SyncService, error) {

	socialConfig := conf.Conf.Socials

	var mainSocial string

	SocialService, err := NewSocialService(socialConfig)
	if err != nil {
		return nil, err
	}

	syncSocials := []string{}

	// get main
	for _, config := range socialConfig {
		if config.Main {
			mainSocial = config.Name
		}
		if config.SyncEnabled && !config.Main {
			syncSocials = append(syncSocials, config.Name)
		}
	}

	return &SyncService{
		mainSocail:    mainSocial,
		socialService: SocialService,
		postDao:       dao,
		socials:       syncSocials,
	}, nil
}

func (s *SyncService) Sync(ctx context.Context) error {
	logger := log.FromContext(ctx)

	mainSocial, err := s.socialService.GetPlatform(s.mainSocail)
	if err != nil {
		return err
	}

	posts, err := mainSocial.Client.ListPosts(ctx, 100)
	if err != nil {
		return err
	}

	for _, post := range posts {
		logger.Info("Processing post", "post_id", post.ID, "content", post.Content[:min(50, len(post.Content))])

		// Skip old posts (older than 1 hour)
		if post.CreatedAt.Before(time.Now().Add(-1 * time.Hour)) {
			logger.Debug("Post is too old, skipping", "post_id", post.ID, "created_at", post.CreatedAt)
			continue
		}

		// Check if post already exists in database
		postModel, err := s.postDao.GetBySocialAndSocialID(ctx, s.mainSocail, post.ID)
		if err != nil {
			logger.Error("Error getting post from database", "error", err, "social", s.mainSocail, "social_id", post.ID)
			continue
		}

		var postID string
		if postModel != nil {
			logger.Debug("Post already exists in database", "post_id", post.ID, "db_id", postModel.ID.Hex())
			postID = postModel.ID.Hex()
		} else {
			// Create new post model and save to database
			logger.Info("Creating new post in database", "post_id", post.ID)
			postModel = &dao.PostModel{
				Social:          s.mainSocail,
				SocialID:        post.ID,
				Content:         post.Content,
				Visibility:      post.Visibility,
				SourcePlatform:  s.mainSocail,
				OriginalID:      post.ID,
				CreatedAt:       post.CreatedAt,
				UpdatedAt:       time.Now(),
				CrossPostStatus: make(map[string]dao.CrossPostStatus),
			}

			postID, err = s.postDao.CreatePost(ctx, postModel)
			if err != nil {
				logger.Error("Error creating post in database", "error", err, "post_id", post.ID)
				continue
			}
			logger.Info("Successfully created post in database", "post_id", post.ID, "db_id", postID)
		}

		// Sync to other platforms
		for _, targetSocial := range s.socials {
			// Check if already synced successfully
			if postModel.CrossPostStatus != nil {
				if status, exists := postModel.CrossPostStatus[targetSocial]; exists && status.Success && status.CrossPosted {
					logger.Debug("Post already synced successfully", "post_id", post.ID, "target_platform", targetSocial)
					continue
				}
			}

			logger.Info("Syncing post to platform", "post_id", post.ID, "target_platform", targetSocial)

			// Get target platform
			targetPlatform, err := s.socialService.GetPlatform(targetSocial)
			if err != nil {
				logger.Error("Error getting target platform", "error", err, "platform", targetSocial)
				// Update cross-post status with error
				status := dao.CrossPostStatus{
					Success:     false,
					Error:       err.Error(),
					CrossPosted: false,
				}
				if updateErr := s.postDao.UpdateCrossPostStatus(ctx, postID, targetSocial, status); updateErr != nil {
					logger.Error("Error updating cross-post status", "error", updateErr, "post_id", postID, "platform", targetSocial)
				}
				continue
			}

			// Post to target platform
			response, err := targetPlatform.Client.Post(ctx, post)
			now := time.Now()

			if err != nil {
				logger.Error("Error posting to platform", "error", err, "post_id", post.ID, "target_platform", targetSocial)
				// Update cross-post status with error
				status := dao.CrossPostStatus{
					Success:     false,
					Error:       err.Error(),
					CrossPosted: false,
					PostedAt:    &now,
				}
				if updateErr := s.postDao.UpdateCrossPostStatus(ctx, postID, targetSocial, status); updateErr != nil {
					logger.Error("Error updating cross-post status", "error", updateErr, "post_id", postID, "platform", targetSocial)
				}
			} else {
				logger.Info("Successfully posted to platform", "post_id", post.ID, "target_platform", targetSocial, "response", response)

				// Extract platform ID from response if available
				var platformID string
				if respMap, ok := response.(map[string]interface{}); ok {
					if id, exists := respMap["id"]; exists {
						if idStr, ok := id.(string); ok {
							platformID = idStr
						}
					}
				}

				// Update cross-post status with success
				status := dao.CrossPostStatus{
					Success:     true,
					PlatformID:  platformID,
					CrossPosted: true,
					PostedAt:    &now,
				}
				if updateErr := s.postDao.UpdateCrossPostStatus(ctx, postID, targetSocial, status); updateErr != nil {
					logger.Error("Error updating cross-post status", "error", updateErr, "post_id", postID, "platform", targetSocial)
				}
			}
		}
	}

	return nil
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
