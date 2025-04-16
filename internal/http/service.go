package http

import (
	"sync"

	"go.orx.me/apps/hyper-sync/internal/conf"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/service"
)

var (
	postService     *service.PostService
	socialService   *service.SocialService
	mongoDAO        *dao.MongoDAO
	serviceInitOnce sync.Once
	serviceInitDone bool
)

// InitServices initializes all services
func InitServices(dao *dao.MongoDAO) {
	serviceInitOnce.Do(func() {
		mongoDAO = dao

		// Initialize social service with configurations from the conf package
		social, err := service.NewSocialService(conf.Conf.Socials)
		if err != nil {
			panic(err)
		}
		socialService = social

		// Initialize post service
		postService = service.NewPostService(dao, socialService)

		serviceInitDone = true
	})
}

// GetPostService returns the post service instance
func GetPostService() *service.PostService {
	if !serviceInitDone {
		return nil
	}
	return postService
}

// GetSocialService returns the social service instance
func GetSocialService() *service.SocialService {
	if !serviceInitDone {
		return nil
	}
	return socialService
}

// GetMongoDAO returns the MongoDB DAO instance
func GetMongoDAO() *dao.MongoDAO {
	if !serviceInitDone {
		return nil
	}
	return mongoDAO
}
