package service

import (
	"context"
	"time"

	"butterfly.orx.me/core/log"
	"github.com/bsm/redislock"
	"go.opentelemetry.io/otel/trace"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/metrics"
	"go.orx.me/apps/hyper-sync/internal/social"
	"go.orx.me/apps/hyper-sync/internal/telemetry"
)

type SyncService struct {
	locker *redislock.Client

	socialService *SocialService
	postDao       dao.PostDao
	metrics       *metrics.SyncMetrics
	tracer        *telemetry.SyncTracer

	mainSocail string
	socials    []string
}

func NewSyncService(dao dao.PostDao, socialService *SocialService, locker *redislock.Client,
	mainSocail string, socials []string) (*SyncService, error) {

	return &SyncService{
		locker:        locker,
		mainSocail:    mainSocail,
		socialService: socialService,
		postDao:       dao,
		socials:       socials,
		metrics:       metrics.NewSyncMetrics(mainSocail),
		tracer:        telemetry.NewSyncTracer(mainSocail),
	}, nil
}

func (s *SyncService) Sync(ctx context.Context) error {
	logger := log.FromContext(ctx)
	lock, err := s.locker.Obtain(ctx, "sync_service", 2*time.Minute, nil)
	if err != nil {
		logger.Info("Failed to obtain lock, skip sync", "error", err)
		return nil
	}
	defer lock.Release(ctx)

	// Start the main sync operation span
	ctx, span := s.tracer.StartSyncOperation(ctx)
	defer span.End()

	return s.metrics.ActiveOperationsContext(ctx, func(ctx context.Context) error {
		return s.metrics.TimedOperationWithContext(ctx, metrics.OperationTotal, func(ctx context.Context) error {
			err := s.doSync(ctx)
			if err != nil {
				s.tracer.SetSpanError(span, err, "sync_operation_failed", map[string]interface{}{
					"main_social": s.mainSocail,
				})
				return err
			}
			s.tracer.SetSpanSuccess(span, map[string]interface{}{
				"main_social": s.mainSocail,
			})
			return nil
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

	// Fetch posts with tracing
	ctx, fetchSpan := s.tracer.StartFetchPosts(ctx, 100)
	var posts []*social.Post
	err = s.metrics.TimedOperationWithContext(ctx, metrics.OperationFetchPosts, func(ctx context.Context) error {
		var fetchErr error
		posts, fetchErr = mainSocial.Client.ListPosts(ctx, 100)
		return fetchErr
	})

	if err != nil {
		s.tracer.SetSpanError(fetchSpan, err, "fetch_posts_failed", map[string]interface{}{
			"platform": s.mainSocail,
		})
		fetchSpan.End()
		s.metrics.IncErrors("", metrics.ErrorTypePlatform)
		return err
	}

	s.tracer.SetSpanSuccess(fetchSpan, map[string]interface{}{
		"posts_count": len(posts),
		"platform":    s.mainSocail,
	})
	fetchSpan.End()

	// Track posts in queue
	s.metrics.SetPostsInQueue(len(posts))

	// Add event to main span
	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		s.tracer.AddEvent(span, "posts_fetched", map[string]interface{}{
			"count":    len(posts),
			"platform": s.mainSocail,
		})
	}

	for _, post := range posts {
		// Start span for processing individual post
		ctx, postSpan := s.tracer.StartProcessPost(ctx, post.ID, post.Content[:min(50, len(post.Content))])

		logger.Info("Processing post", "post_id", post.ID, "content", post.Content[:min(50, len(post.Content))])

		// Skip old posts (older than 1 hour)
		if post.CreatedAt.Before(time.Now().Add(-1 * time.Hour)) {
			logger.Debug("Post is too old, skipping", "post_id", post.ID, "created_at", post.CreatedAt)
			s.metrics.IncPostsProcessed(metrics.StatusSkippedOld)
			s.tracer.SetSpanSkipped(postSpan, "post_too_old", map[string]interface{}{
				"post_age_hours": time.Since(post.CreatedAt).Hours(),
				"created_at":     post.CreatedAt.Format(time.RFC3339),
			})
			postSpan.End()
			continue
		}

		// if post is private, skip
		if post.Visibility == social.VisibilityLevelPrivate {
			logger.Info("Post is private, skipping", "post_id", post.ID)
			s.metrics.IncPostsProcessed(metrics.StatusSkippedPrivate)
			s.tracer.SetSpanSkipped(postSpan, "post_private", nil)
			postSpan.End()
			continue
		}

		// Check if post already exists in database
		ctx, dbSpan := s.tracer.StartDatabaseOperation(ctx, "get_post", post.ID)
		postModel, err := s.postDao.GetBySocialAndSocialID(ctx, s.mainSocail, post.ID)
		if err != nil {
			logger.Error("Error getting post from database", "error", err, "social", s.mainSocail, "social_id", post.ID)
			s.metrics.IncDatabaseOps(metrics.OperationGetPost, metrics.StatusError)
			s.metrics.IncErrors("", metrics.ErrorTypeDatabase)
			s.tracer.SetSpanError(dbSpan, err, "database_get_error", nil)
			dbSpan.End()
			s.tracer.SetSpanError(postSpan, err, "post_processing_failed", nil)
			postSpan.End()
			continue
		}
		s.metrics.IncDatabaseOps(metrics.OperationGetPost, metrics.StatusSuccess)
		s.tracer.SetSpanSuccess(dbSpan, map[string]interface{}{
			"post_exists": postModel != nil,
		})
		dbSpan.End()

		var postID string
		if postModel != nil {
			logger.Info("Post already exists in database", "post_id", post.ID, "db_id", postModel.ID.Hex())
			postID = postModel.ID.Hex()
			s.metrics.IncPostsProcessed(metrics.StatusExists)

			s.tracer.AddEvent(postSpan, "post_exists", map[string]interface{}{
				"db_id": postModel.ID.Hex(),
			})
		} else {
			// Create new post model and save to database
			logger.Info("Creating new post in database", "post_id", post.ID)

			ctx, createSpan := s.tracer.StartDatabaseOperation(ctx, "create_post", post.ID)
			postModel = dao.FromSocialPost(post)
			// Override specific fields for sync service
			postModel.Social = s.mainSocail
			postModel.SocialID = post.ID
			postModel.SourcePlatform = s.mainSocail
			postModel.OriginalID = post.ID
			postModel.CreatedAt = post.CreatedAt
			postModel.UpdatedAt = time.Now()
			postModel.CrossPostStatus = make(map[string]dao.CrossPostStatus)

			postID, err = s.postDao.CreatePost(ctx, postModel)
			if err != nil {
				logger.Error("Error creating post in database", "error", err, "post_id", post.ID)
				s.metrics.IncDatabaseOps(metrics.OperationCreatePost, metrics.StatusError)
				s.metrics.IncErrors("", metrics.ErrorTypeDatabase)
				s.tracer.SetSpanError(createSpan, err, "database_create_error", nil)
				createSpan.End()
				s.tracer.SetSpanError(postSpan, err, "post_processing_failed", nil)
				postSpan.End()
				continue
			}
			s.metrics.IncDatabaseOps(metrics.OperationCreatePost, metrics.StatusSuccess)
			logger.Info("Successfully created post in database", "post_id", post.ID, "db_id", postID)
			s.metrics.IncPostsProcessed(metrics.StatusProcessed)

			s.tracer.SetSpanSuccess(createSpan, map[string]interface{}{
				"db_id": postID,
			})
			createSpan.End()

			s.tracer.AddEvent(postSpan, "post_created", map[string]interface{}{
				"db_id": postID,
			})
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

			// Start cross-post span
			ctx, crossPostSpan := s.tracer.StartCrossPost(ctx, post.ID, targetSocial)

			// Get target platform
			targetPlatform, err := s.socialService.GetPlatform(targetSocial)
			if err != nil {
				logger.Error("Error getting target platform", "error", err, "platform", targetSocial)
				s.metrics.IncErrors(targetSocial, metrics.ErrorTypePlatform)
				s.metrics.IncCrossPosts(targetSocial, metrics.StatusError)

				s.tracer.SetSpanError(crossPostSpan, err, "platform_get_error", map[string]interface{}{
					"target_platform": targetSocial,
				})

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
				crossPostSpan.End()
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

				s.tracer.SetSpanError(crossPostSpan, err, "cross_post_failed", map[string]interface{}{
					"target_platform": targetSocial,
				})

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

				s.tracer.SetSpanSuccess(crossPostSpan, map[string]interface{}{
					"target_platform": targetSocial,
					"platform_id":     platformID,
				})

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
			crossPostSpan.End()
		}

		// Mark post processing as complete
		s.tracer.SetSpanSuccess(postSpan, map[string]interface{}{
			"post_id":          post.ID,
			"platforms_synced": len(s.socials),
		})
		postSpan.End()
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
