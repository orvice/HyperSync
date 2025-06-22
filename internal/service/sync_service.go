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
}

func NewSyncService(dao dao.PostDao) (*SyncService, error) {

	socialConfig := conf.Conf.Socials

	var mainSocial string

	SocialService, err := NewSocialService(socialConfig)
	if err != nil {
		return nil, err
	}

	// get main
	for _, config := range socialConfig {
		if config.Main {
			mainSocial = config.Name
		}
	}

	return &SyncService{
		mainSocail:    mainSocial,
		socialService: SocialService,
		postDao:       dao,
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
		logger.Info("Syncing post", "post", post)

		if post.CreatedAt.Before(time.Now().Add(-1 * time.Hour)) {
			logger.Info("Post is too old", "post", post)
			continue
		}

		postModel, err := s.postDao.GetBySocialAndSocialID(ctx, s.mainSocail, post.ID)
		if err != nil {
			logger.Error("Error getting post",
				"error", err)
			continue
		}

		if postModel != nil {
			logger.Info("Post already exists", "post", post)

		} else {
			logger.Info("Post not found", "post", post)
		}

	}

	return nil
}
