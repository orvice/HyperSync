package server

import (
	"context"

	"go.orx.me/apps/hyper-sync/internal/service"
	"go.orx.me/apps/hyper-sync/internal/social"
	pb "go.orx.me/apps/hyper-sync/pkg/proto/api/v1"
)

// PostServer implements the gRPC HyperSyncService server interface
type PostServer struct {
	pb.UnimplementedHyperSyncServiceServer
	postService *service.PostService
}

// NewPostServer creates a new instance of PostServer
func NewPostServer(postService *service.PostService) *PostServer {
	return &PostServer{
		postService: postService,
	}
}

// CreatePost implements the CreatePost RPC method
func (s *PostServer) CreatePost(ctx context.Context, req *pb.CreatePostRequest) (*pb.CreatePostResponse, error) {
	// Convert request to social.Post
	post := &social.Post{
		Content:        req.Content,
		Visibility:     req.Visibility,
		SourcePlatform: req.SourcePlatform,
		OriginalID:     req.OriginalId,
	}

	// Convert media IDs to Media objects
	for _, mediaID := range req.MediaIds {
		post.Media = append(post.Media, *social.NewMediaFromURL(mediaID))
	}

	// Create post
	postID, err := s.postService.CreatePost(ctx, post, nil) // TODO: Add platform list from config
	if err != nil {
		return nil, err
	}

	return &pb.CreatePostResponse{
		Id: postID,
	}, nil
}

// GetPost implements the GetPost RPC method
func (s *PostServer) GetPost(ctx context.Context, req *pb.GetPostRequest) (*pb.GetPostResponse, error) {
	post, err := s.postService.GetPost(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &pb.GetPostResponse{
		Post: convertToProtoPost(post),
	}, nil
}

// ListPosts implements the ListPosts RPC method
func (s *PostServer) ListPosts(ctx context.Context, req *pb.ListPostsRequest) (*pb.ListPostsResponse, error) {
	// List posts
	posts, err := s.postService.ListPosts(ctx, req.SourcePlatform, int(req.Limit), int(req.Skip/req.Limit+1))
	if err != nil {
		return nil, err
	}

	// Convert to response
	protoPosts := make([]*pb.Post, 0, len(posts))
	for _, post := range posts {
		protoPosts = append(protoPosts, convertToProtoPost(post))
	}

	return &pb.ListPostsResponse{
		Posts: protoPosts,
	}, nil
}

// DeletePost implements the DeletePost RPC method
func (s *PostServer) DeletePost(ctx context.Context, req *pb.DeletePostRequest) (*pb.DeletePostResponse, error) {
	err := s.postService.DeletePost(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &pb.DeletePostResponse{
		Success: true,
	}, nil
}

// Helper function to convert social.Post to proto.Post
func convertToProtoPost(post *social.Post) *pb.Post {
	if post == nil {
		return nil
	}

	protoPost := &pb.Post{
		Id:             post.ID,
		Content:        post.Content,
		Visibility:     post.Visibility,
		SourcePlatform: post.SourcePlatform,
		OriginalId:     post.OriginalID,
	}

	// Convert media to media IDs (using URLs as IDs)
	mediaIds := make([]string, 0, len(post.Media))
	for range post.Media {
		// Since we can't access the URL directly, we'll use a placeholder
		// In a real implementation, we should add a GetURL() method to the Media struct
		mediaIds = append(mediaIds, "media-placeholder")
	}
	protoPost.MediaIds = mediaIds

	return protoPost
}
