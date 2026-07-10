package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.orx.me/apps/hyper-sync/internal/post"
	"go.orx.me/apps/hyper-sync/internal/service"
	"go.orx.me/apps/hyper-sync/internal/social"
)

type mockSocialClient struct {
	name        string
	postErr     error
	postCalls   []*social.Post
	updateCalls []updateCall
	updateErr   error
}

type updateCall struct {
	platformID string
	post       *social.Post
}

func (m *mockSocialClient) Post(_ context.Context, p *social.Post) (interface{}, error) {
	m.postCalls = append(m.postCalls, p)
	if m.postErr != nil {
		return nil, m.postErr
	}
	return map[string]string{"id": "platform-post-123"}, nil
}

func (m *mockSocialClient) ListPosts(_ context.Context, _ int) ([]*social.Post, error) {
	return nil, nil
}

func (m *mockSocialClient) Name() string {
	return m.name
}

func (m *mockSocialClient) Update(_ context.Context, platformID string, p *social.Post) error {
	m.updateCalls = append(m.updateCalls, updateCall{platformID: platformID, post: p})
	return m.updateErr
}

func setupPublishWorkerTest(t *testing.T, clients map[string]social.SocialClient, opts ...service.PublishWorkerOption) (*service.PublishWorker, *post.MemoryStore) {
	t.Helper()
	store := post.NewMemoryStore()
	worker := service.NewPublishWorker(store, nil, clients, 3, opts...)
	return worker, store
}

func TestPublishWorker_PublishesPendingPost(t *testing.T) {
	mastodon := &mockSocialClient{name: "mastodon"}
	clients := map[string]social.SocialClient{"mastodon": mastodon}

	worker, store := setupPublishWorkerTest(t, clients)

	// Create a published post with mastodon as sync target
	p := &post.Post{
		Content:     "Hello world",
		Visibility:  "public",
		Status:      "published",
		SyncPending: true,
		SyncTargets: []string{"mastodon"},
		CrossPostStatus: map[string]post.CrossPostStatus{
			"mastodon": {Success: false},
		},
	}
	created, err := store.Create(context.Background(), p)
	require.NoError(t, err)

	// Run the worker
	err = worker.Run(context.Background())
	require.NoError(t, err)

	// Verify the platform client was called
	require.Len(t, mastodon.postCalls, 1)
	assert.Equal(t, "Hello world", mastodon.postCalls[0].Content)

	// Verify the post's CrossPostStatus was updated
	updated, err := store.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	assert.True(t, updated.CrossPostStatus["mastodon"].Success)
	assert.NotEmpty(t, updated.CrossPostStatus["mastodon"].PlatformID)
}

func TestPublishWorker_SkipsPrivateAndDirectPosts(t *testing.T) {
	mastodon := &mockSocialClient{name: "mastodon"}
	clients := map[string]social.SocialClient{"mastodon": mastodon}

	worker, store := setupPublishWorkerTest(t, clients)

	for _, vis := range []string{"private", "direct"} {
		_, err := store.Create(context.Background(), &post.Post{
			Content:     "secret " + vis,
			Visibility:  vis,
			Status:      "published",
			SyncPending: true,
			SyncTargets: []string{"mastodon"},
			CrossPostStatus: map[string]post.CrossPostStatus{
				"mastodon": {Success: false},
			},
		})
		require.NoError(t, err)
	}

	err := worker.Run(context.Background())
	require.NoError(t, err)

	// No calls should have been made
	assert.Empty(t, mastodon.postCalls)
}

func TestPublishWorker_RetriesFailedPostsUpToMax(t *testing.T) {
	mastodon := &mockSocialClient{name: "mastodon", postErr: errors.New("network error")}
	clients := map[string]social.SocialClient{"mastodon": mastodon}

	worker, store := setupPublishWorkerTest(t, clients)

	// Post with retryCount < max → should be attempted
	_, err := store.Create(context.Background(), &post.Post{
		Content:     "will retry",
		Visibility:  "public",
		Status:      "published",
		SyncPending: true,
		SyncTargets: []string{"mastodon"},
		CrossPostStatus: map[string]post.CrossPostStatus{
			"mastodon": {Success: false, RetryCount: 2},
		},
	})
	require.NoError(t, err)

	// Post with retryCount >= max → should NOT be attempted
	_, err = store.Create(context.Background(), &post.Post{
		Content:     "exhausted retries",
		Visibility:  "public",
		Status:      "published",
		SyncPending: true,
		SyncTargets: []string{"mastodon"},
		CrossPostStatus: map[string]post.CrossPostStatus{
			"mastodon": {Success: false, RetryCount: 3},
		},
	})
	require.NoError(t, err)

	err = worker.Run(context.Background())
	require.NoError(t, err)

	// Only the first post (retryCount=2 < max=3) should be attempted
	assert.Len(t, mastodon.postCalls, 1)
	assert.Equal(t, "will retry", mastodon.postCalls[0].Content)
}

func TestPublishWorker_SkipsAlreadySyncedPlatforms(t *testing.T) {
	mastodon := &mockSocialClient{name: "mastodon"}
	bluesky := &mockSocialClient{name: "bluesky"}
	clients := map[string]social.SocialClient{"mastodon": mastodon, "bluesky": bluesky}

	worker, store := setupPublishWorkerTest(t, clients)

	_, err := store.Create(context.Background(), &post.Post{
		Content:     "partial sync",
		Visibility:  "public",
		Status:      "published",
		SyncPending: true,
		SyncTargets: []string{"mastodon", "bluesky"},
		CrossPostStatus: map[string]post.CrossPostStatus{
			"mastodon": {Success: true, PlatformID: "already-done"},
			"bluesky":  {Success: false},
		},
	})
	require.NoError(t, err)

	err = worker.Run(context.Background())
	require.NoError(t, err)

	// Mastodon should NOT be called (already synced)
	assert.Empty(t, mastodon.postCalls)
	// Bluesky should be called
	assert.Len(t, bluesky.postCalls, 1)
}

func TestPublishWorker_NeedsUpdate_CallsUpdate(t *testing.T) {
	mastodon := &mockSocialClient{name: "mastodon"}
	clients := map[string]social.SocialClient{"mastodon": mastodon}

	worker, store := setupPublishWorkerTest(t, clients)

	_, err := store.Create(context.Background(), &post.Post{
		Content:     "Updated content",
		Visibility:  "public",
		Status:      "published",
		SyncPending: true,
		SyncTargets: []string{"mastodon"},
		CrossPostStatus: map[string]post.CrossPostStatus{
			"mastodon": {Success: true, PlatformID: "masto-123", NeedsUpdate: true},
		},
	})
	require.NoError(t, err)

	err = worker.Run(context.Background())
	require.NoError(t, err)

	// Should call Update, not Post
	assert.Empty(t, mastodon.postCalls)
	require.Len(t, mastodon.updateCalls, 1)
	assert.Equal(t, "masto-123", mastodon.updateCalls[0].platformID)
	assert.Equal(t, "Updated content", mastodon.updateCalls[0].post.Content)
}

func TestPublishWorker_NeedsUpdate_ClearsFlag(t *testing.T) {
	mastodon := &mockSocialClient{name: "mastodon"}
	clients := map[string]social.SocialClient{"mastodon": mastodon}

	worker, store := setupPublishWorkerTest(t, clients)

	created, err := store.Create(context.Background(), &post.Post{
		Content:     "Updated content",
		Visibility:  "public",
		Status:      "published",
		SyncPending: true,
		SyncTargets: []string{"mastodon"},
		CrossPostStatus: map[string]post.CrossPostStatus{
			"mastodon": {Success: true, PlatformID: "masto-123", NeedsUpdate: true},
		},
	})
	require.NoError(t, err)

	err = worker.Run(context.Background())
	require.NoError(t, err)

	// Verify the NeedsUpdate flag is cleared
	updated, err := store.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	assert.False(t, updated.CrossPostStatus["mastodon"].NeedsUpdate)
	assert.True(t, updated.CrossPostStatus["mastodon"].Success)
}

func TestPublishWorker_DeletingPost_AllPlatformDeletesSucceed_RemovesPost(t *testing.T) {
	deleter := &mockPlatformDeleter{results: map[string]error{}}
	worker, store := setupPublishWorkerTest(t, nil, service.WithWorkerDeleter(deleter))

	created, err := store.Create(context.Background(), &post.Post{
		Content:    "Being deleted",
		Visibility: "public",
		Status:     "deleting",
		SyncPending: true,
		CrossPostStatus: map[string]post.CrossPostStatus{
			"mastodon": {PlatformID: "masto-111", RetryCount: 1},
		},
	})
	require.NoError(t, err)

	err = worker.Run(context.Background())
	require.NoError(t, err)

	// Platform delete should have been called
	assert.Contains(t, deleter.calls, deleteCall{platform: "mastodon", platformID: "masto-111"})

	// Post should be deleted locally
	_, err = store.GetByID(context.Background(), created.ID)
	assert.ErrorIs(t, err, post.ErrNotFound)
}

func TestPublishWorker_DeletingPost_PlatformDeleteFails_IncrementsRetryCount(t *testing.T) {
	deleter := &mockPlatformDeleter{results: map[string]error{
		"mastodon": fmt.Errorf("still failing"),
	}}
	worker, store := setupPublishWorkerTest(t, nil, service.WithWorkerDeleter(deleter))

	created, err := store.Create(context.Background(), &post.Post{
		Content:    "Being deleted",
		Visibility: "public",
		Status:     "deleting",
		SyncPending: true,
		CrossPostStatus: map[string]post.CrossPostStatus{
			"mastodon": {PlatformID: "masto-111", RetryCount: 1},
		},
	})
	require.NoError(t, err)

	err = worker.Run(context.Background())
	require.NoError(t, err)

	// Post should still exist in "deleting" state
	got, err := store.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, "deleting", got.Status)
	assert.Equal(t, 2, got.CrossPostStatus["mastodon"].RetryCount)
	assert.Equal(t, "masto-111", got.CrossPostStatus["mastodon"].PlatformID)
}

func TestPublishWorker_DeletingPost_MixedRetry_PersistsSuccessfulRemoval(t *testing.T) {
	deleter := &mockPlatformDeleter{results: map[string]error{
		"bluesky": fmt.Errorf("still failing"),
	}}
	worker, store := setupPublishWorkerTest(t, nil, service.WithWorkerDeleter(deleter))

	created, err := store.Create(context.Background(), &post.Post{
		Content:    "Mixed retry",
		Visibility: "public",
		Status:     "deleting",
		SyncPending: true,
		CrossPostStatus: map[string]post.CrossPostStatus{
			"mastodon": {PlatformID: "masto-111", RetryCount: 1},
			"bluesky":  {PlatformID: "bsky-222", RetryCount: 1},
		},
	})
	require.NoError(t, err)

	err = worker.Run(context.Background())
	require.NoError(t, err)

	// Post should still exist (bluesky failed)
	got, err := store.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, "deleting", got.Status)

	// Mastodon succeeded — must NOT appear in persisted CrossPostStatus
	_, hasMasto := got.CrossPostStatus["mastodon"]
	assert.False(t, hasMasto, "successfully deleted platform should be removed from store")

	// Bluesky failed — must be preserved with incremented retry
	assert.Equal(t, 2, got.CrossPostStatus["bluesky"].RetryCount)
}

func TestPublishWorker_DeletingPost_RetriesExhausted_RemovesPost(t *testing.T) {
	deleter := &mockPlatformDeleter{results: map[string]error{
		"mastodon": fmt.Errorf("permanently broken"),
	}}
	worker, store := setupPublishWorkerTest(t, nil, service.WithWorkerDeleter(deleter))

	created, err := store.Create(context.Background(), &post.Post{
		Content:    "Exhausted retries",
		Visibility: "public",
		Status:     "deleting",
		SyncPending: true,
		CrossPostStatus: map[string]post.CrossPostStatus{
			"mastodon": {PlatformID: "masto-111", RetryCount: 2},
		},
	})
	require.NoError(t, err)

	err = worker.Run(context.Background())
	require.NoError(t, err)

	// Post should be deleted locally even though platform delete failed
	// (retries exhausted: max=3, count was 2, now 3)
	_, err = store.GetByID(context.Background(), created.ID)
	assert.ErrorIs(t, err, post.ErrNotFound)
}

func TestPublishWorker_NeedsDelete_Succeeds_RemovesEntry(t *testing.T) {
	deleter := &mockPlatformDeleter{results: map[string]error{}}
	mastodon := &mockSocialClient{name: "mastodon"}
	clients := map[string]social.SocialClient{"mastodon": mastodon}
	worker, store := setupPublishWorkerTest(t, clients, service.WithWorkerDeleter(deleter))

	created, err := store.Create(context.Background(), &post.Post{
		Content:     "Still published",
		Visibility:  "public",
		Status:      "published",
		SyncPending: true,
		SyncTargets: []string{"mastodon"},
		CrossPostStatus: map[string]post.CrossPostStatus{
			"mastodon": {Success: true, PlatformID: "masto-123"},
			"bluesky":  {Success: true, PlatformID: "bsky-456", NeedsDelete: true, RetryCount: 1},
		},
	})
	require.NoError(t, err)

	err = worker.Run(context.Background())
	require.NoError(t, err)

	// Platform delete should have been called for bluesky
	assert.Contains(t, deleter.calls, deleteCall{platform: "bluesky", platformID: "bsky-456"})

	// Bluesky entry should be removed from the post
	got, err := store.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	_, hasBsky := got.CrossPostStatus["bluesky"]
	assert.False(t, hasBsky, "successfully deleted NeedsDelete entry should be removed")

	// Mastodon should be untouched
	assert.True(t, got.CrossPostStatus["mastodon"].Success)
}

func TestPublishWorker_NeedsDelete_Fails_IncrementsRetryCount(t *testing.T) {
	deleter := &mockPlatformDeleter{results: map[string]error{
		"bluesky": fmt.Errorf("still failing"),
	}}
	mastodon := &mockSocialClient{name: "mastodon"}
	clients := map[string]social.SocialClient{"mastodon": mastodon}
	worker, store := setupPublishWorkerTest(t, clients, service.WithWorkerDeleter(deleter))

	created, err := store.Create(context.Background(), &post.Post{
		Content:     "Still published",
		Visibility:  "public",
		Status:      "published",
		SyncPending: true,
		SyncTargets: []string{"mastodon"},
		CrossPostStatus: map[string]post.CrossPostStatus{
			"mastodon": {Success: true, PlatformID: "masto-123"},
			"bluesky":  {Success: true, PlatformID: "bsky-456", NeedsDelete: true, RetryCount: 1},
		},
	})
	require.NoError(t, err)

	err = worker.Run(context.Background())
	require.NoError(t, err)

	got, err := store.GetByID(context.Background(), created.ID)
	require.NoError(t, err)

	// Bluesky should still exist with incremented retry count
	bsky := got.CrossPostStatus["bluesky"]
	assert.True(t, bsky.NeedsDelete)
	assert.Equal(t, 2, bsky.RetryCount)
	assert.Equal(t, "bsky-456", bsky.PlatformID)
}

func TestPublishWorker_NeedsDelete_LastTargetRemoved_StillProcessed(t *testing.T) {
	deleter := &mockPlatformDeleter{results: map[string]error{}}
	clients := map[string]social.SocialClient{}
	worker, store := setupPublishWorkerTest(t, clients, service.WithWorkerDeleter(deleter))

	// Post where the only target was removed, immediate delete failed,
	// so SyncTargets is empty but NeedsDelete entry remains.
	created, err := store.Create(context.Background(), &post.Post{
		Content:     "All targets removed",
		Visibility:  "public",
		Status:      "published",
		SyncPending: true,
		SyncTargets: []string{},
		CrossPostStatus: map[string]post.CrossPostStatus{
			"bluesky": {Success: false, PlatformID: "bsky-456", NeedsDelete: true, RetryCount: 1},
		},
	})
	require.NoError(t, err)

	err = worker.Run(context.Background())
	require.NoError(t, err)

	// The worker must have processed this post despite empty SyncTargets
	assert.Contains(t, deleter.calls, deleteCall{platform: "bluesky", platformID: "bsky-456"})

	got, err := store.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	_, hasBsky := got.CrossPostStatus["bluesky"]
	assert.False(t, hasBsky, "NeedsDelete entry should be removed after successful delete")
	assert.False(t, got.SyncPending, "SyncPending should be cleared after all NeedsDelete entries resolved")
}

func TestPublishWorker_NeedsDelete_RetriesExhausted_RemovesEntry(t *testing.T) {
	deleter := &mockPlatformDeleter{results: map[string]error{
		"bluesky": fmt.Errorf("permanently broken"),
	}}
	mastodon := &mockSocialClient{name: "mastodon"}
	clients := map[string]social.SocialClient{"mastodon": mastodon}
	worker, store := setupPublishWorkerTest(t, clients, service.WithWorkerDeleter(deleter))

	created, err := store.Create(context.Background(), &post.Post{
		Content:     "Still published",
		Visibility:  "public",
		Status:      "published",
		SyncPending: true,
		SyncTargets: []string{"mastodon"},
		CrossPostStatus: map[string]post.CrossPostStatus{
			"mastodon": {Success: true, PlatformID: "masto-123"},
			"bluesky":  {Success: false, PlatformID: "bsky-456", NeedsDelete: true, RetryCount: 2},
		},
	})
	require.NoError(t, err)

	err = worker.Run(context.Background())
	require.NoError(t, err)

	got, err := store.GetByID(context.Background(), created.ID)
	require.NoError(t, err)

	// Bluesky should be removed — retries exhausted (count was 2, now 3 = max)
	_, hasBsky := got.CrossPostStatus["bluesky"]
	assert.False(t, hasBsky, "NeedsDelete entry with exhausted retries should be cleaned up")

	// Mastodon untouched
	assert.True(t, got.CrossPostStatus["mastodon"].Success)
}
