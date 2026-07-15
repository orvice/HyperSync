package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"butterfly.orx.me/core/log"
	"github.com/bsm/redislock"
	"go.opentelemetry.io/otel/trace"
	"go.orx.me/apps/hyper-sync/internal/conf"
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

	mainSocial string
	socials    []string
}

func NewSyncService(dao dao.PostDao, socialService *SocialService, locker *redislock.Client,
	mainSocial string, socials []string) (*SyncService, error) {

	return &SyncService{
		locker:        locker,
		mainSocial:    mainSocial,
		socialService: socialService,
		postDao:       dao,
		socials:       socials,
		metrics:       metrics.NewSyncMetrics(mainSocial),
		tracer:        telemetry.NewSyncTracer(mainSocial),
	}, nil
}

func (s *SyncService) Sync(ctx context.Context) error {
	logger := log.FromContext(ctx)
	lockKey := fmt.Sprintf("sync_service:%s", s.mainSocial)
	const lockTTL = 2 * time.Minute
	lock, err := s.locker.Obtain(ctx, lockKey, lockTTL, nil)
	if err != nil {
		if errors.Is(err, redislock.ErrNotObtained) {
			logger.Info("Lock held by another instance, skip sync", "lock_key", lockKey)
		} else {
			logger.Error("Failed to obtain lock, skip sync", "lock_key", lockKey, "error", err)
		}
		return nil
	}
	// 即使 ctx 在关停时已被取消，也要确保锁能正常释放
	defer lock.Release(context.WithoutCancel(ctx))

	// 后台看门狗：定期续期锁，防止 doSync 运行超过 TTL 后锁被其他实例抢占，
	// 从而导致重复发帖。在 doSync 返回时停止续期。
	stopRefresh := make(chan struct{})
	defer close(stopRefresh)
	go func() {
		ticker := time.NewTicker(lockTTL / 2)
		defer ticker.Stop()
		for {
			select {
			case <-stopRefresh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := lock.Refresh(ctx, lockTTL, nil); err != nil {
					logger.Warn("Failed to refresh sync lock", "lock_key", lockKey, "error", err)
				}
			}
		}
	}()

	// Start the main sync operation span
	ctx, span := s.tracer.StartSyncOperation(ctx)
	defer span.End()

	return s.metrics.ActiveOperationsContext(ctx, func(ctx context.Context) error {
		return s.metrics.TimedOperationWithContext(ctx, metrics.OperationTotal, func(ctx context.Context) error {
			err := s.doSync(ctx)
			if err != nil {
				s.tracer.SetSpanError(span, err, "sync_operation_failed", map[string]interface{}{
					"main_social": s.mainSocial,
				})
				return err
			}
			s.tracer.SetSpanSuccess(span, map[string]interface{}{
				"main_social": s.mainSocial,
			})
			return nil
		})
	})
}

func (s *SyncService) doSync(ctx context.Context) error {
	logger := log.FromContext(ctx)

	mainSocial, err := s.socialService.GetPlatform(s.mainSocial)
	if err != nil {
		s.metrics.IncErrors("", metrics.ErrorTypePlatform)
		return err
	}

	batchSize := 100
	if conf.Conf.Sync != nil && conf.Conf.Sync.BatchSize > 0 {
		batchSize = conf.Conf.Sync.BatchSize
	}

	// Fetch posts with tracing
	ctx, fetchSpan := s.tracer.StartFetchPosts(ctx, batchSize)
	var posts []*social.Post
	err = s.metrics.TimedOperationWithContext(ctx, metrics.OperationFetchPosts, func(ctx context.Context) error {
		var fetchErr error
		posts, fetchErr = mainSocial.Client.ListPosts(ctx, batchSize)
		return fetchErr
	})

	if err != nil {
		s.tracer.SetSpanError(fetchSpan, err, "fetch_posts_failed", map[string]interface{}{
			"platform": s.mainSocial,
		})
		fetchSpan.End()
		s.metrics.IncErrors("", metrics.ErrorTypePlatform)
		return err
	}

	s.tracer.SetSpanSuccess(fetchSpan, map[string]interface{}{
		"posts_count": len(posts),
		"platform":    s.mainSocial,
	})
	fetchSpan.End()

	// Track posts in queue
	s.metrics.SetPostsInQueue(len(posts))

	// Add event to main span
	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		s.tracer.AddEvent(span, "posts_fetched", map[string]interface{}{
			"count":    len(posts),
			"platform": s.mainSocial,
		})
	}

	skipOlder := time.Hour
	if conf.Conf.Sync != nil && conf.Conf.Sync.SkipOlder > 0 {
		skipOlder = conf.Conf.Sync.SkipOlder
	}

	maxRetries := 3
	if conf.Conf.Sync != nil && conf.Conf.Sync.MaxRetries > 0 {
		maxRetries = conf.Conf.Sync.MaxRetries
	}

	// Collect posts that are too recent so we can requeue them for
	// buffer-based clients (e.g. Telegram) where ListPosts is destructive.
	var delayedPosts []*social.Post
	if requeuer, ok := mainSocial.Client.(social.PostRequeuer); ok {
		defer func() {
			if len(delayedPosts) > 0 {
				requeuer.Requeue(delayedPosts)
				logger.Info("requeued delayed posts", "count", len(delayedPosts))
			}
		}()
	}

	for _, post := range posts {
		contentPreview := preview(post.Content, 50)

		// Start span for processing individual post
		ctx, postSpan := s.tracer.StartProcessPost(ctx, post.ID, contentPreview)

		logger.Info("Processing post", "post_id", post.ID, "content", contentPreview)

		// Skip old posts
		if post.CreatedAt.Before(time.Now().Add(-skipOlder)) {
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
		if post.Visibility == social.VisibilityLevelDirect {
			logger.Info("Post is direct, skipping", "post_id", post.ID)
			s.metrics.IncPostsProcessed(metrics.StatusSkippedDirect)
			s.tracer.SetSpanSkipped(postSpan, "post_direct", nil)
			postSpan.End()
			continue
		}

		if mainSocial.Config.SyncDelay > 0 && time.Since(post.CreatedAt) < mainSocial.Config.SyncDelay {
			logger.Info("Post too recent, delaying sync",
				"post_id", post.ID, "age", time.Since(post.CreatedAt), "sync_delay", mainSocial.Config.SyncDelay)
			s.metrics.IncPostsProcessed(metrics.StatusSkippedOld)
			s.tracer.SetSpanSkipped(postSpan, "post_too_recent", map[string]interface{}{
				"post_age_seconds": time.Since(post.CreatedAt).Seconds(),
				"sync_delay":       mainSocial.Config.SyncDelay.String(),
			})
			postSpan.End()
			delayedPosts = append(delayedPosts, post)
			continue
		}

		// Check if post already exists in database
		ctx, dbSpan := s.tracer.StartDatabaseOperation(ctx, "get_post", post.ID)
		postModel, err := s.postDao.GetBySocialAndSocialID(ctx, s.mainSocial, post.ID)
		if err != nil {
			logger.Error("Error getting post from database", "error", err, "social", s.mainSocial, "social_id", post.ID)
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
			postModel.Social = s.mainSocial
			postModel.SocialID = post.ID
			postModel.SourcePlatform = s.mainSocial
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
			// Check existing cross-post status
			retryCount := 0
			if postModel.CrossPostStatus != nil {
				if status, exists := postModel.CrossPostStatus[targetSocial]; exists {
					if status.Success && status.CrossPosted {
						logger.Info("Post already synced successfully",
							"post_id", post.ID, "target_platform", targetSocial)
						continue
					}
					// 失败重试已达上限，放弃以避免无限重试
					if status.RetryCount >= maxRetries {
						logger.Warn("Post cross-post retries exhausted, giving up",
							"post_id", post.ID, "target_platform", targetSocial, "retry_count", status.RetryCount)
						continue
					}
					retryCount = status.RetryCount
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
					RetryCount:  retryCount + 1,
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
					RetryCount:  retryCount + 1,
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
				platformID := extractPlatformID(response)

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

// preview returns a rune-safe content preview.
func preview(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes])
}

// extractPlatformID extracts a platform-specific post ID from common response shapes.
func extractPlatformID(response interface{}) string {
	if response == nil {
		return ""
	}

	if respMap, ok := response.(map[string]interface{}); ok {
		for _, key := range []string{"id", "uri", "rkey", "cid"} {
			if value, exists := respMap[key]; exists {
				if id := stringifyPlatformID(value); id != "" {
					return id
				}
			}
		}
	}

	value := reflect.Indirect(reflect.ValueOf(response))
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return ""
	}

	for _, fieldName := range []string{"ID", "Id", "URI", "Uri", "RKey", "Rkey", "CID", "Cid"} {
		field := value.FieldByName(fieldName)
		if field.IsValid() && field.CanInterface() {
			if id := stringifyPlatformID(field.Interface()); id != "" {
				return id
			}
		}
	}

	return ""
}

func stringifyPlatformID(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}
