package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.orx.me/apps/hyper-sync/internal/post"
	v1 "go.orx.me/apps/hyper-sync/pkg/proto/api/v1"
)

// PlatformDeleter handles deleting posts from external platforms.
type PlatformDeleter interface {
	DeleteFromPlatform(ctx context.Context, platform, platformID string) error
}

type PostServiceOption func(*PostService)

func WithPlatformDeleter(d PlatformDeleter) PostServiceOption {
	return func(s *PostService) {
		s.deleter = d
	}
}

type PostService struct {
	store   post.Store
	deleter PlatformDeleter
}

func NewPostService(store post.Store, opts ...PostServiceOption) *PostService {
	s := &PostService{store: store}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *PostService) CreatePost(ctx context.Context, req *connect.Request[v1.CreatePostRequest]) (*connect.Response[v1.CreatePostResponse], error) {
	p := &post.Post{
		Content:     req.Msg.Content,
		Visibility:  req.Msg.Visibility,
		Status:      req.Msg.Status,
		MediaIDs:    req.Msg.MediaIds,
		SyncTargets: req.Msg.SyncTargets,
	}

	if p.Status == "" {
		p.Status = "draft"
	}
	if p.Visibility == "" {
		p.Visibility = "public"
	}
	if err := validatePostFields(p.Status, p.Visibility); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	p.SyncPending = p.Status == "published" && len(p.SyncTargets) > 0

	created, err := s.store.Create(ctx, p)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&v1.CreatePostResponse{
		Post: postToProto(created),
	}), nil
}

func (s *PostService) GetPost(ctx context.Context, req *connect.Request[v1.GetPostRequest]) (*connect.Response[v1.GetPostResponse], error) {
	p, err := s.store.GetByID(ctx, req.Msg.Id)
	if err != nil {
		if errors.Is(err, post.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&v1.GetPostResponse{
		Post: postToProto(p),
	}), nil
}

func (s *PostService) ListPosts(ctx context.Context, req *connect.Request[v1.ListPostsRequest]) (*connect.Response[v1.ListPostsResponse], error) {
	result, err := s.store.List(ctx, post.ListOptions{
		PageSize: int(req.Msg.PageSize),
		Page:     int(req.Msg.Page),
		Status:   req.Msg.Status,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	posts := make([]*v1.Post, 0, len(result.Posts))
	for _, p := range result.Posts {
		posts = append(posts, postToProto(p))
	}

	return connect.NewResponse(&v1.ListPostsResponse{
		Posts: posts,
		Total: int32(result.Total),
	}), nil
}

func (s *PostService) UpdatePost(ctx context.Context, req *connect.Request[v1.UpdatePostRequest]) (*connect.Response[v1.UpdatePostResponse], error) {
	existing, err := s.store.GetByID(ctx, req.Msg.Id)
	if err != nil {
		if errors.Is(err, post.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if existing.Status == "deleting" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("cannot update a post that is being deleted"))
	}

	visibility := req.Msg.Visibility
	if visibility == "" {
		visibility = existing.Visibility
	}
	if err := validatePostFields(existing.Status, visibility); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	contentChanged := existing.Content != req.Msg.Content ||
		existing.Visibility != visibility ||
		!slices.Equal(existing.MediaIDs, req.Msg.MediaIds)
	targetsChanged := !slices.Equal(existing.SyncTargets, req.Msg.SyncTargets)

	existing.Content = req.Msg.Content
	existing.Visibility = visibility
	existing.MediaIDs = req.Msg.MediaIds
	existing.SyncTargets = req.Msg.SyncTargets

	// Mark already-synced platforms as needs_update when content changes on a
	// published post. Resetting the retry budget also gives permanently failed
	// platforms a fresh chance after an edit.
	if existing.Status == "published" && contentChanged {
		for platform, status := range existing.CrossPostStatus {
			if status.Success && status.PlatformID != "" {
				status.NeedsUpdate = true
			}
			status.RetryCount = 0
			existing.CrossPostStatus[platform] = status
		}
	}
	if existing.Status == "published" && (contentChanged || targetsChanged) && len(existing.SyncTargets) > 0 {
		existing.SyncPending = true
	}

	updated, err := s.store.Update(ctx, existing)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&v1.UpdatePostResponse{
		Post: postToProto(updated),
	}), nil
}

func (s *PostService) PublishPost(ctx context.Context, req *connect.Request[v1.PublishPostRequest]) (*connect.Response[v1.PublishPostResponse], error) {
	existing, err := s.store.GetByID(ctx, req.Msg.Id)
	if err != nil {
		if errors.Is(err, post.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if existing.Status == "published" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("post is already published"))
	}
	if existing.Status == "deleting" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("cannot publish a post that is being deleted"))
	}

	existing.Status = "published"
	existing.SyncPending = len(existing.SyncTargets) > 0
	updated, err := s.store.Update(ctx, existing)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&v1.PublishPostResponse{
		Post: postToProto(updated),
	}), nil
}

func (s *PostService) DeletePost(ctx context.Context, req *connect.Request[v1.DeletePostRequest]) (*connect.Response[v1.DeletePostResponse], error) {
	existing, err := s.store.GetByID(ctx, req.Msg.Id)
	if err != nil {
		if errors.Is(err, post.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// If already in "deleting", re-attempt the cascade for any entries that
	// still have a PlatformID (regardless of Success flag).
	if existing.Status == "deleting" {
		return s.retryCascadeDelete(ctx, existing)
	}

	// Cascade delete to platforms — track which ones fail so we can retry.
	var failedPlatforms []string
	if s.deleter != nil {
		for platform, status := range existing.CrossPostStatus {
			if status.Success && status.PlatformID != "" {
				if err := s.deleter.DeleteFromPlatform(ctx, platform, status.PlatformID); err != nil {
					slog.Error("cascade delete from platform failed", "platform", platform, "platform_id", status.PlatformID, "error", err)
					failedPlatforms = append(failedPlatforms, platform)
				}
			}
		}
	}

	if len(failedPlatforms) > 0 {
		existing.Status = "deleting"
		existing.SyncPending = true
		for _, platform := range failedPlatforms {
			cs := existing.CrossPostStatus[platform]
			cs.Success = false
			cs.Error = "pending platform delete"
			cs.RetryCount++
			existing.CrossPostStatus[platform] = cs
		}
		for platform, status := range existing.CrossPostStatus {
			if status.Success {
				delete(existing.CrossPostStatus, platform)
			}
		}
		if _, err := s.store.Update(ctx, existing); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		return connect.NewResponse(&v1.DeletePostResponse{}), nil
	}

	if err := s.store.Delete(ctx, req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&v1.DeletePostResponse{}), nil
}

func (s *PostService) retryCascadeDelete(ctx context.Context, existing *post.Post) (*connect.Response[v1.DeletePostResponse], error) {
	anyFailed := false
	if s.deleter != nil {
		for platform, status := range existing.CrossPostStatus {
			if status.PlatformID == "" {
				continue
			}
			if err := s.deleter.DeleteFromPlatform(ctx, platform, status.PlatformID); err != nil {
				slog.Error("cascade delete from platform failed", "platform", platform, "platform_id", status.PlatformID, "error", err)
				status.RetryCount++
				status.Error = err.Error()
				existing.CrossPostStatus[platform] = status
				anyFailed = true
			} else {
				delete(existing.CrossPostStatus, platform)
			}
		}
	}

	if !anyFailed || len(existing.CrossPostStatus) == 0 {
		if err := s.store.Delete(ctx, existing.ID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	} else {
		if _, err := s.store.Update(ctx, existing); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	return connect.NewResponse(&v1.DeletePostResponse{}), nil
}

var (
	validStatuses   = map[string]bool{"draft": true, "published": true}
	validVisibility = map[string]bool{"public": true, "unlisted": true, "private": true, "direct": true}
)

func validatePostFields(status, visibility string) error {
	if !validStatuses[status] {
		return fmt.Errorf("invalid status %q", status)
	}
	if !validVisibility[visibility] {
		return fmt.Errorf("invalid visibility %q", visibility)
	}
	return nil
}

func postToProto(p *post.Post) *v1.Post {
	proto := &v1.Post{
		Id:              p.ID,
		Content:         p.Content,
		Visibility:      p.Visibility,
		Status:          p.Status,
		MediaIds:        p.MediaIDs,
		SyncTargets:     p.SyncTargets,
		CreatedAt:       timestamppb.New(p.CreatedAt),
		UpdatedAt:       timestamppb.New(p.UpdatedAt),
		CrossPostStatus: make(map[string]*v1.CrossPostStatus),
	}

	for platform, status := range p.CrossPostStatus {
		cs := &v1.CrossPostStatus{
			Success:     status.Success,
			Error:       status.Error,
			PlatformId:  status.PlatformID,
			RetryCount:  int32(status.RetryCount),
			NeedsUpdate: status.NeedsUpdate,
		}
		if status.PostedAt != nil {
			cs.PostedAt = timestamppb.New(*status.PostedAt)
		}
		proto.CrossPostStatus[platform] = cs
	}

	return proto
}
