package service_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.orx.me/apps/hyper-sync/internal/post"
	"go.orx.me/apps/hyper-sync/internal/service"
	v1 "go.orx.me/apps/hyper-sync/pkg/proto/api/v1"
	"go.orx.me/apps/hyper-sync/pkg/proto/api/v1/v1connect"
)

func setupPostTest(t *testing.T) (v1connect.PostServiceClient, func()) {
	t.Helper()

	store := post.NewMemoryStore()
	svc := service.NewPostService(store)

	mux := http.NewServeMux()
	path, handler := v1connect.NewPostServiceHandler(svc)
	mux.Handle(path, handler)

	server := httptest.NewServer(mux)
	client := v1connect.NewPostServiceClient(server.Client(), server.URL)

	return client, server.Close
}

func TestCreatePost_Draft_ReturnsPost(t *testing.T) {
	client, cleanup := setupPostTest(t)
	defer cleanup()

	resp, err := client.CreatePost(context.Background(), connect.NewRequest(&v1.CreatePostRequest{
		Content:    "Hello world",
		Visibility: "public",
		Status:     "draft",
	}))

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Msg.Post.Id)
	assert.Equal(t, "Hello world", resp.Msg.Post.Content)
	assert.Equal(t, "public", resp.Msg.Post.Visibility)
	assert.Equal(t, "draft", resp.Msg.Post.Status)
}

func TestCreatePost_Published_ReturnsPost(t *testing.T) {
	client, cleanup := setupPostTest(t)
	defer cleanup()

	resp, err := client.CreatePost(context.Background(), connect.NewRequest(&v1.CreatePostRequest{
		Content:     "Published post",
		Visibility:  "public",
		Status:      "published",
		SyncTargets: []string{"mastodon", "bluesky"},
	}))

	require.NoError(t, err)
	assert.Equal(t, "published", resp.Msg.Post.Status)
	assert.Equal(t, []string{"mastodon", "bluesky"}, resp.Msg.Post.SyncTargets)
}

func TestGetPost_Exists_ReturnsPost(t *testing.T) {
	client, cleanup := setupPostTest(t)
	defer cleanup()

	created, err := client.CreatePost(context.Background(), connect.NewRequest(&v1.CreatePostRequest{
		Content:    "Test post",
		Visibility: "public",
		Status:     "draft",
	}))
	require.NoError(t, err)

	resp, err := client.GetPost(context.Background(), connect.NewRequest(&v1.GetPostRequest{
		Id: created.Msg.Post.Id,
	}))

	require.NoError(t, err)
	assert.Equal(t, "Test post", resp.Msg.Post.Content)
}

func TestGetPost_NotFound_ReturnsError(t *testing.T) {
	client, cleanup := setupPostTest(t)
	defer cleanup()

	_, err := client.GetPost(context.Background(), connect.NewRequest(&v1.GetPostRequest{
		Id: "nonexistent",
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestListPosts_FilterByStatus(t *testing.T) {
	client, cleanup := setupPostTest(t)
	defer cleanup()

	// Create drafts and published
	for i := 0; i < 3; i++ {
		_, err := client.CreatePost(context.Background(), connect.NewRequest(&v1.CreatePostRequest{
			Content:    "draft post",
			Visibility: "public",
			Status:     "draft",
		}))
		require.NoError(t, err)
	}
	for i := 0; i < 2; i++ {
		_, err := client.CreatePost(context.Background(), connect.NewRequest(&v1.CreatePostRequest{
			Content:    "published post",
			Visibility: "public",
			Status:     "published",
		}))
		require.NoError(t, err)
	}

	resp, err := client.ListPosts(context.Background(), connect.NewRequest(&v1.ListPostsRequest{
		Status: "draft",
	}))

	require.NoError(t, err)
	assert.Equal(t, int32(3), resp.Msg.Total)
	assert.Len(t, resp.Msg.Posts, 3)
}

func TestPublishPost_DraftToPublished(t *testing.T) {
	client, cleanup := setupPostTest(t)
	defer cleanup()

	created, err := client.CreatePost(context.Background(), connect.NewRequest(&v1.CreatePostRequest{
		Content:     "Draft to publish",
		Visibility:  "public",
		Status:      "draft",
		SyncTargets: []string{"mastodon"},
	}))
	require.NoError(t, err)
	assert.Equal(t, "draft", created.Msg.Post.Status)

	resp, err := client.PublishPost(context.Background(), connect.NewRequest(&v1.PublishPostRequest{
		Id: created.Msg.Post.Id,
	}))

	require.NoError(t, err)
	assert.Equal(t, "published", resp.Msg.Post.Status)
}

func TestPublishPost_AlreadyPublished_ReturnsError(t *testing.T) {
	client, cleanup := setupPostTest(t)
	defer cleanup()

	created, err := client.CreatePost(context.Background(), connect.NewRequest(&v1.CreatePostRequest{
		Content:    "Already published",
		Visibility: "public",
		Status:     "published",
	}))
	require.NoError(t, err)

	_, err = client.PublishPost(context.Background(), connect.NewRequest(&v1.PublishPostRequest{
		Id: created.Msg.Post.Id,
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestUpdatePost_PublishedPost_MarksSyncedPlatformsAsNeedsUpdate(t *testing.T) {
	store := post.NewMemoryStore()
	svc := service.NewPostService(store)

	mux := http.NewServeMux()
	path, handler := v1connect.NewPostServiceHandler(svc)
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	defer server.Close()
	client := v1connect.NewPostServiceClient(server.Client(), server.URL)

	// Simulate a published post that was already synced to mastodon and bluesky
	p, err := store.Create(context.Background(), &post.Post{
		Content:     "Original content",
		Visibility:  "public",
		Status:      "published",
		SyncTargets: []string{"mastodon", "bluesky"},
		CrossPostStatus: map[string]post.CrossPostStatus{
			"mastodon": {Success: true, PlatformID: "masto-123"},
			"bluesky":  {Success: true, PlatformID: "bsky-456"},
		},
	})
	require.NoError(t, err)

	// Update the post content
	resp, err := client.UpdatePost(context.Background(), connect.NewRequest(&v1.UpdatePostRequest{
		Id:          p.ID,
		Content:     "Updated content",
		Visibility:  "public",
		SyncTargets: []string{"mastodon", "bluesky"},
	}))

	require.NoError(t, err)
	assert.Equal(t, "Updated content", resp.Msg.Post.Content)

	// Both platforms should now be marked as needs_update
	assert.True(t, resp.Msg.Post.CrossPostStatus["mastodon"].NeedsUpdate)
	assert.True(t, resp.Msg.Post.CrossPostStatus["bluesky"].NeedsUpdate)
	// PlatformID should be preserved
	assert.Equal(t, "masto-123", resp.Msg.Post.CrossPostStatus["mastodon"].PlatformId)
	assert.Equal(t, "bsky-456", resp.Msg.Post.CrossPostStatus["bluesky"].PlatformId)
}

func TestUpdatePost_DraftPost_DoesNotMarkNeedsUpdate(t *testing.T) {
	client, cleanup := setupPostTest(t)
	defer cleanup()

	created, err := client.CreatePost(context.Background(), connect.NewRequest(&v1.CreatePostRequest{
		Content:     "Draft post",
		Visibility:  "public",
		Status:      "draft",
		SyncTargets: []string{"mastodon"},
	}))
	require.NoError(t, err)

	resp, err := client.UpdatePost(context.Background(), connect.NewRequest(&v1.UpdatePostRequest{
		Id:          created.Msg.Post.Id,
		Content:     "Updated draft",
		Visibility:  "public",
		SyncTargets: []string{"mastodon"},
	}))

	require.NoError(t, err)
	assert.Equal(t, "Updated draft", resp.Msg.Post.Content)
	// No CrossPostStatus should exist for a draft
	assert.Empty(t, resp.Msg.Post.CrossPostStatus)
}

func TestUpdatePost_DeletingPost_RejectsUpdate(t *testing.T) {
	store := post.NewMemoryStore()
	svc := service.NewPostService(store)

	mux := http.NewServeMux()
	path, handler := v1connect.NewPostServiceHandler(svc)
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	defer server.Close()
	client := v1connect.NewPostServiceClient(server.Client(), server.URL)

	p, err := store.Create(context.Background(), &post.Post{
		Content:    "Being deleted",
		Visibility: "public",
		Status:     "deleting",
		CrossPostStatus: map[string]post.CrossPostStatus{
			"mastodon": {PlatformID: "masto-111"},
		},
	})
	require.NoError(t, err)

	_, err = client.UpdatePost(context.Background(), connect.NewRequest(&v1.UpdatePostRequest{
		Id:         p.ID,
		Content:    "Trying to update",
		Visibility: "public",
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestPublishPost_DeletingPost_RejectsPublish(t *testing.T) {
	store := post.NewMemoryStore()
	svc := service.NewPostService(store)

	mux := http.NewServeMux()
	path, handler := v1connect.NewPostServiceHandler(svc)
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	defer server.Close()
	client := v1connect.NewPostServiceClient(server.Client(), server.URL)

	p, err := store.Create(context.Background(), &post.Post{
		Content:    "Being deleted",
		Visibility: "public",
		Status:     "deleting",
	})
	require.NoError(t, err)

	_, err = client.PublishPost(context.Background(), connect.NewRequest(&v1.PublishPostRequest{
		Id: p.ID,
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestDeletePost_CascadesToPlatforms(t *testing.T) {
	store := post.NewMemoryStore()
	deleter := &mockPlatformDeleter{results: map[string]error{}}
	svc := service.NewPostService(store, service.WithPlatformDeleter(deleter))

	mux := http.NewServeMux()
	path, handler := v1connect.NewPostServiceHandler(svc)
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	defer server.Close()
	client := v1connect.NewPostServiceClient(server.Client(), server.URL)

	// Create a published post that was synced to mastodon and bluesky
	p, err := store.Create(context.Background(), &post.Post{
		Content:     "To be deleted",
		Visibility:  "public",
		Status:      "published",
		SyncTargets: []string{"mastodon", "bluesky"},
		CrossPostStatus: map[string]post.CrossPostStatus{
			"mastodon": {Success: true, PlatformID: "masto-999"},
			"bluesky":  {Success: true, PlatformID: "bsky-888"},
		},
	})
	require.NoError(t, err)

	_, err = client.DeletePost(context.Background(), connect.NewRequest(&v1.DeletePostRequest{
		Id: p.ID,
	}))
	require.NoError(t, err)

	// Verify platform deletions were called
	assert.Contains(t, deleter.calls, deleteCall{platform: "mastodon", platformID: "masto-999"})
	assert.Contains(t, deleter.calls, deleteCall{platform: "bluesky", platformID: "bsky-888"})

	// Verify post is deleted from store
	_, err = store.GetByID(context.Background(), p.ID)
	assert.ErrorIs(t, err, post.ErrNotFound)
}

func TestDeletePost_PlatformFailure_TransitionsToDeleting(t *testing.T) {
	store := post.NewMemoryStore()
	deleter := &mockPlatformDeleter{results: map[string]error{
		"mastodon": fmt.Errorf("network error"),
	}}
	svc := service.NewPostService(store, service.WithPlatformDeleter(deleter))

	mux := http.NewServeMux()
	path, handler := v1connect.NewPostServiceHandler(svc)
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	defer server.Close()
	client := v1connect.NewPostServiceClient(server.Client(), server.URL)

	p, err := store.Create(context.Background(), &post.Post{
		Content:     "Delete even if platform fails",
		Visibility:  "public",
		Status:      "published",
		SyncTargets: []string{"mastodon"},
		CrossPostStatus: map[string]post.CrossPostStatus{
			"mastodon": {Success: true, PlatformID: "masto-111"},
		},
	})
	require.NoError(t, err)

	_, err = client.DeletePost(context.Background(), connect.NewRequest(&v1.DeletePostRequest{
		Id: p.ID,
	}))
	require.NoError(t, err)

	// Post should NOT be deleted — it should transition to "deleting"
	got, err := store.GetByID(context.Background(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, "deleting", got.Status)
	assert.True(t, got.SyncPending)

	// PlatformID must be preserved so the worker can retry
	ms := got.CrossPostStatus["mastodon"]
	assert.Equal(t, "masto-111", ms.PlatformID)
	assert.False(t, ms.Success, "success should be cleared so worker retries the delete")
	assert.Equal(t, 1, ms.RetryCount)
}

func TestDeletePost_MixedPlatformFailure_OnlyFailedPlatformNeedsRetry(t *testing.T) {
	store := post.NewMemoryStore()
	deleter := &mockPlatformDeleter{results: map[string]error{
		"mastodon": fmt.Errorf("network error"),
	}}
	svc := service.NewPostService(store, service.WithPlatformDeleter(deleter))

	mux := http.NewServeMux()
	path, handler := v1connect.NewPostServiceHandler(svc)
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	defer server.Close()
	client := v1connect.NewPostServiceClient(server.Client(), server.URL)

	p, err := store.Create(context.Background(), &post.Post{
		Content:     "Mixed results",
		Visibility:  "public",
		Status:      "published",
		SyncTargets: []string{"mastodon", "bluesky"},
		CrossPostStatus: map[string]post.CrossPostStatus{
			"mastodon": {Success: true, PlatformID: "masto-555"},
			"bluesky":  {Success: true, PlatformID: "bsky-666"},
		},
	})
	require.NoError(t, err)

	_, err = client.DeletePost(context.Background(), connect.NewRequest(&v1.DeletePostRequest{
		Id: p.ID,
	}))
	require.NoError(t, err)

	got, err := store.GetByID(context.Background(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, "deleting", got.Status)

	// Mastodon failed — should be preserved with retry info
	ms := got.CrossPostStatus["mastodon"]
	assert.Equal(t, "masto-555", ms.PlatformID)
	assert.False(t, ms.Success)
	assert.Equal(t, 1, ms.RetryCount)

	// Bluesky succeeded — should be removed from CrossPostStatus
	_, hasBsky := got.CrossPostStatus["bluesky"]
	assert.False(t, hasBsky, "successfully deleted platform should be removed")
}

func TestDeletePost_AlreadyDeleting_RetriesInsteadOfOrphaning(t *testing.T) {
	store := post.NewMemoryStore()
	deleter := &mockPlatformDeleter{results: map[string]error{
		"mastodon": fmt.Errorf("still failing"),
	}}
	svc := service.NewPostService(store, service.WithPlatformDeleter(deleter))

	mux := http.NewServeMux()
	path, handler := v1connect.NewPostServiceHandler(svc)
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	defer server.Close()
	client := v1connect.NewPostServiceClient(server.Client(), server.URL)

	// Simulate a post already in "deleting" state (from a previous failed delete)
	p, err := store.Create(context.Background(), &post.Post{
		Content:    "Already deleting",
		Visibility: "public",
		Status:     "deleting",
		SyncPending: true,
		CrossPostStatus: map[string]post.CrossPostStatus{
			"mastodon": {Success: false, PlatformID: "masto-222", RetryCount: 1},
		},
	})
	require.NoError(t, err)

	_, err = client.DeletePost(context.Background(), connect.NewRequest(&v1.DeletePostRequest{
		Id: p.ID,
	}))
	require.NoError(t, err)

	// Post must NOT be deleted — it should stay in "deleting" with preserved PlatformID
	got, err := store.GetByID(context.Background(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, "deleting", got.Status)
	assert.Equal(t, "masto-222", got.CrossPostStatus["mastodon"].PlatformID)
}

type deleteCall struct {
	platform   string
	platformID string
}

type mockPlatformDeleter struct {
	calls   []deleteCall
	results map[string]error
}

func (m *mockPlatformDeleter) DeleteFromPlatform(ctx context.Context, platform, platformID string) error {
	m.calls = append(m.calls, deleteCall{platform: platform, platformID: platformID})
	return m.results[platform]
}
