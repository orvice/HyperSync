package service

import (
	"context"
	"time"

	"butterfly.orx.me/core/log"
	"go.orx.me/apps/hyper-sync/internal/conf"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/metrics"
	"go.orx.me/apps/hyper-sync/internal/social"
)

type SyncService struct {
	socialService *SocialService
	postDao       dao.PostDao
	metrics       *metrics.SyncMetrics

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
			syncSocials = append(syncSocials, config.SyncTo...)
		}
	}

	return &SyncService{
		mainSocail:    mainSocial,
		socialService: SocialService,
		postDao:       dao,
		socials:       syncSocials,
		metrics:       metrics.NewSyncMetrics(mainSocial),
	}, nil
}

func (s *SyncService) Sync(ctx context.Context) error {
	return s.metrics.ActiveOperationsContext(ctx, func(ctx context.Context) error {
		return s.metrics.TimedOperationWithContext(ctx, metrics.OperationTotal, func(ctx context.Context) error {
			return s.doSync(ctx)
		})
	})
}

func (s *SyncService) doSync(ctx context.Context) error {
	logger := log.FromContext(ctx)

	mainSocial, err := s.socialService.GetPlatform(s.mainSocail)
	if err != nil {
		s.metrics.IncErrors("", metrics.ErrorTypePlatform)
		return err
	}

	var posts []*social.Post
	err = s.metrics.TimedOperationWithContext(ctx, metrics.OperationFetchPosts, func(ctx context.Context) error {
		var fetchErr error
		posts, fetchErr = mainSocial.Client.ListPosts(ctx, 100)
		return fetchErr
	})

	if err != nil {
		s.metrics.IncErrors("", metrics.ErrorTypePlatform)
		return err
	}

	// Track posts in queue
	s.metrics.SetPostsInQueue(len(posts))

	for _, post := range posts {
		logger.Info("Processing post", "post_id", post.ID, "content", post.Content[:min(50, len(post.Content))])

		// Skip old posts (older than 1 hour)
		if post.CreatedAt.Before(time.Now().Add(-1 * time.Hour)) {
			logger.Debug("Post is too old, skipping", "post_id", post.ID, "created_at", post.CreatedAt)
			s.metrics.IncPostsProcessed(metrics.StatusSkippedOld)
			continue
		}

		// Check if post already exists in database
		postModel, err := s.postDao.GetBySocialAndSocialID(ctx, s.mainSocail, post.ID)
		if err != nil {
			logger.Error("Error getting post from database", "error", err, "social", s.mainSocail, "social_id", post.ID)
			s.metrics.IncDatabaseOps(metrics.OperationGetPost, metrics.StatusError)
			s.metrics.IncErrors("", metrics.ErrorTypeDatabase)
			continue
		}
		s.metrics.IncDatabaseOps(metrics.OperationGetPost, metrics.StatusSuccess)

		var postID string
		if postModel != nil {
			logger.Info("Post already exists in database", "post_id", post.ID, "db_id", postModel.ID.Hex())
			postID = postModel.ID.Hex()
			s.metrics.IncPostsProcessed(metrics.StatusExists)
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
				s.metrics.IncDatabaseOps(metrics.OperationCreatePost, metrics.StatusError)
				s.metrics.IncErrors("", metrics.ErrorTypeDatabase)
				continue
			}
			s.metrics.IncDatabaseOps(metrics.OperationCreatePost, metrics.StatusSuccess)
			logger.Info("Successfully created post in database", "post_id", post.ID, "db_id", postID)
			s.metrics.IncPostsProcessed(metrics.StatusProcessed)
		}

		logger.Info("start to sync to other platforms",
			"platforms", s.socials)

		// Sync to other platforms
		for _, targetSocial := range s.socials {
			// Check if already synced successfully
			if postModel.CrossPostStatus != nil {
				if status, exists := postModel.CrossPostStatus[targetSocial]; exists && status.Success && status.CrossPosted {
					logger.Info("Post already synced successfully",
						"post_id", post.ID, "target_platform", targetSocial)
					continue
				}
			}

			logger.Info("Syncing post to platform", "post_id", post.ID, "target_platform", targetSocial)

			// Get target platform
			targetPlatform, err := s.socialService.GetPlatform(targetSocial)
			if err != nil {
				logger.Error("Error getting target platform", "error", err, "platform", targetSocial)
				s.metrics.IncErrors(targetSocial, metrics.ErrorTypePlatform)
				s.metrics.IncCrossPosts(targetSocial, metrics.StatusError)

				// Update cross-post status with error
				status := dao.CrossPostStatus{
					Success:     false,
					Error:       err.Error(),
					CrossPosted: false,
				}
				if updateErr := s.postDao.UpdateCrossPostStatus(ctx, postID, targetSocial, status); updateErr != nil {
					logger.Error("Error updating cross-post status", "error", updateErr, "post_id", postID, "platform", targetSocial)
					s.metrics.IncDatabaseOps(metrics.OperationUpdateStatus, metrics.StatusError)
				} else {
					s.metrics.IncDatabaseOps(metrics.OperationUpdateStatus, metrics.StatusSuccess)
				}
				continue
			}

			// Post to target platform with timing
			var response interface{}
			err = s.metrics.TimedOperationWithContext(ctx, metrics.OperationSyncToPlatform, func(ctx context.Context) error {
				var postErr error
				response, postErr = targetPlatform.Client.Post(ctx, post)
				return postErr
			})

			now := time.Now()

			if err != nil {
				logger.Error("Error posting to platform", "error", err, "post_id", post.ID, "target_platform", targetSocial)
				s.metrics.IncErrors(targetSocial, metrics.ErrorTypePlatform)
				s.metrics.IncCrossPosts(targetSocial, metrics.StatusError)

				// Update cross-post status with error
				status := dao.CrossPostStatus{
					Success:     false,
					Error:       err.Error(),
					CrossPosted: false,
					PostedAt:    &now,
				}
				if updateErr := s.postDao.UpdateCrossPostStatus(ctx, postID, targetSocial, status); updateErr != nil {
					logger.Error("Error updating cross-post status", "error", updateErr, "post_id", postID, "platform", targetSocial)
					s.metrics.IncDatabaseOps(metrics.OperationUpdateStatus, metrics.StatusError)
				} else {
					s.metrics.IncDatabaseOps(metrics.OperationUpdateStatus, metrics.StatusSuccess)
				}
			} else {
				logger.Info("Successfully posted to platform", "post_id", post.ID, "target_platform", targetSocial, "response", response)
				s.metrics.IncCrossPosts(targetSocial, metrics.StatusSuccess)

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
					s.metrics.IncDatabaseOps(metrics.OperationUpdateStatus, metrics.StatusError)
				} else {
					s.metrics.IncDatabaseOps(metrics.OperationUpdateStatus, metrics.StatusSuccess)
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
