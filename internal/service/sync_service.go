package service

import (
	"context"

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
	}

	return nil
}
