package api

import (
	"context"

	"go.orx.me/apps/hyper-sync/internal/service"
	v1 "go.orx.me/apps/hyper-sync/pkg/proto/api/v1"
)

type HyperSyncAPI struct {
	svc *service.HyperSyncService
}

func NewHyperSyncAPI(svc *service.HyperSyncService) *HyperSyncAPI {
	return &HyperSyncAPI{
		svc: svc,
	}
}

func (a *HyperSyncAPI) ListPosts(ctx context.Context, req *v1.ListPostsRequest) (*v1.ListPostsResponse, error) {
	return a.svc.ListPosts(ctx, req)
}

func (a *HyperSyncAPI) GetPost(ctx context.Context, req *v1.GetPostRequest) (*v1.GetPostResponse, error) {
	return a.svc.GetPost(ctx, req)
}

func (a *HyperSyncAPI) CreatePost(ctx context.Context, req *v1.CreatePostRequest) (*v1.CreatePostResponse, error) {
	return a.svc.CreatePost(ctx, req)
}

func (a *HyperSyncAPI) DeletePost(ctx context.Context, req *v1.DeletePostRequest) (*v1.DeletePostResponse, error) {
	return a.svc.DeletePost(ctx, req)
}
