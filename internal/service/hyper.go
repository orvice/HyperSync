package service

import (
	"context"

	"go.orx.me/apps/hyper-sync/internal/dao"
	v1 "go.orx.me/apps/hyper-sync/pkg/proto/api/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type HyperSyncService struct {
	dao *dao.MongoDAO
}

func NewHyperSyncService(dao *dao.MongoDAO) *HyperSyncService {
	return &HyperSyncService{
		dao: dao,
	}
}

func (s *HyperSyncService) ListPosts(ctx context.Context, req *v1.ListPostsRequest) (*v1.ListPostsResponse, error) {
	filter := make(map[string]interface{})
	if req.SourcePlatform != "" {
		filter["source_platform"] = req.SourcePlatform
	}
	if req.Visibility != "" {
		filter["visibility"] = req.Visibility
	}

	posts, err := s.dao.ListPosts(ctx, filter, req.Limit, req.Skip)
	if err != nil {
		return nil, err
	}

	resp := &v1.ListPostsResponse{
		Posts: make([]*v1.Post, 0, len(posts)),
	}

	for _, post := range posts {
		resp.Posts = append(resp.Posts, convertPostModelToProto(post))
	}

	return resp, nil
}

func (s *HyperSyncService) GetPost(ctx context.Context, req *v1.GetPostRequest) (*v1.GetPostResponse, error) {
	post, err := s.dao.GetPostByID(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	if post == nil {
		return &v1.GetPostResponse{}, nil
	}

	return &v1.GetPostResponse{
		Post: convertPostModelToProto(post),
	}, nil
}

func (s *HyperSyncService) CreatePost(ctx context.Context, req *v1.CreatePostRequest) (*v1.CreatePostResponse, error) {
	post := &dao.PostModel{
		Content:        req.Content,
		Visibility:     req.Visibility,
		SourcePlatform: req.SourcePlatform,
		OriginalID:     req.OriginalId,
		MediaIDs:       req.MediaIds,
	}

	id, err := s.dao.CreatePost(ctx, post)
	if err != nil {
		return nil, err
	}

	return &v1.CreatePostResponse{
		Id: id,
	}, nil
}

func (s *HyperSyncService) DeletePost(ctx context.Context, req *v1.DeletePostRequest) (*v1.DeletePostResponse, error) {
	err := s.dao.DeletePost(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &v1.DeletePostResponse{
		Success: true,
	}, nil
}

func convertPostModelToProto(post *dao.PostModel) *v1.Post {
	protoPost := &v1.Post{
		Id:              post.ID.Hex(),
		Content:         post.Content,
		Visibility:      post.Visibility,
		SourcePlatform:  post.SourcePlatform,
		OriginalId:      post.OriginalID,
		MediaIds:        post.MediaIDs,
		CreatedAt:       timestamppb.New(post.CreatedAt),
		UpdatedAt:       timestamppb.New(post.UpdatedAt),
		CrossPostStatus: make(map[string]*v1.CrossPostStatus),
	}

	for platform, status := range post.CrossPostStatus {
		protoPost.CrossPostStatus[platform] = &v1.CrossPostStatus{
			Success:     status.Success,
			Error:       status.Error,
			PlatformId:  status.PlatformID,
			CrossPosted: status.CrossPosted,
		}
		if status.PostedAt != nil {
			protoPost.CrossPostStatus[platform].PostedAt = timestamppb.New(*status.PostedAt)
		}
	}

	return protoPost
}
