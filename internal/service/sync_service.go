package service

import "go.orx.me/apps/hyper-sync/internal/dao"

type SyncService struct {
	mainSocialService *SocialService
	postDao           dao.PostDao
}

func NewSyncService() *SyncService {
	return &SyncService{}
}
