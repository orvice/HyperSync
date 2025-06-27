package dao

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"go.orx.me/apps/hyper-sync/internal/social"
)

// setupTestDB initializes a test MongoDB connection
func setupTestDB(t *testing.T) (PostDao, func()) {
	// Connect to MongoDB
	ctx := context.Background()
	clientOptions := options.Client().ApplyURI("mongodb://localhost:27017")
	client, err := mongo.Connect(clientOptions)
	require.NoError(t, err)

	// Create a unique database name for this test run
	dbName := "hyper_sync_test_" + time.Now().Format("20060102150405")
	dao := NewMongoDAO(client)

	// Return cleanup function
	cleanup := func() {
		err := client.Database(dbName).Drop(ctx)
		require.NoError(t, err)
		err = client.Disconnect(ctx)
		require.NoError(t, err)
	}

	return dao, cleanup
}

// createTestPost creates a test post
func createTestPost() *social.Post {
	return &social.Post{
		Content:        "Test content",
		Visibility:     social.VisibilityLevelPublic,
		SourcePlatform: "test_platform",
		OriginalID:     "test_original_id",
	}
}

func TestFromSocialPost(t *testing.T) {
	post := createTestPost()
	model := FromSocialPost(post)

	assert.Equal(t, post.Content, model.Content)
	assert.Equal(t, post.Visibility.String(), model.Visibility)
	assert.Equal(t, post.SourcePlatform, model.SourcePlatform)
	assert.Equal(t, post.OriginalID, model.OriginalID)
	assert.NotZero(t, model.CreatedAt)
	assert.NotZero(t, model.UpdatedAt)
	assert.NotNil(t, model.CrossPostStatus)
}

func TestPostModel_ToSocialPost(t *testing.T) {
	originalPost := createTestPost()
	model := FromSocialPost(originalPost)
	model.ID = bson.NewObjectID()

	convertedPost := model.ToSocialPost()

	assert.Equal(t, model.ID.Hex(), convertedPost.ID)
	assert.Equal(t, model.Content, convertedPost.Content)
	assert.Equal(t, model.Visibility, convertedPost.Visibility.String())
	assert.Equal(t, model.SourcePlatform, convertedPost.SourcePlatform)
	assert.Equal(t, model.OriginalID, convertedPost.OriginalID)
}

func TestMongoDAO_CreateAndGetPost(t *testing.T) {
	dao, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	post := FromSocialPost(createTestPost())

	// Test Create
	id, err := dao.CreatePost(ctx, post)
	require.NoError(t, err)
	assert.NotEmpty(t, id)

	// Test GetPostByID
	retrieved, err := dao.GetPostByID(ctx, id)
	require.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, post.Content, retrieved.Content)
	assert.Equal(t, post.SourcePlatform, retrieved.SourcePlatform)
	assert.Equal(t, post.OriginalID, retrieved.OriginalID)

	// Test GetPostByOriginalID
	byOriginalID, err := dao.GetPostByOriginalID(ctx, post.SourcePlatform, post.OriginalID)
	require.NoError(t, err)
	assert.NotNil(t, byOriginalID)
	assert.Equal(t, post.Content, byOriginalID.Content)
}

func TestMongoDAO_ListPosts(t *testing.T) {
	dao, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple posts
	for i := 0; i < 5; i++ {
		postModel := FromSocialPost(createTestPost())
		postModel.Content = "Content " + time.Now().String()
		_, err := dao.CreatePost(ctx, postModel)
		require.NoError(t, err)
	}

	// Test listing all posts
	posts, err := dao.ListPosts(ctx, nil, 10, 0)
	require.NoError(t, err)
	assert.Len(t, posts, 5)

	// Test with limit
	limitedPosts, err := dao.ListPosts(ctx, nil, 2, 0)
	require.NoError(t, err)
	assert.Len(t, limitedPosts, 2)

	// Test with skip
	skippedPosts, err := dao.ListPosts(ctx, nil, 10, 2)
	require.NoError(t, err)
	assert.Len(t, skippedPosts, 3)

	// Test with filter
	testPost := FromSocialPost(createTestPost())
	testPost.SourcePlatform = "special_platform"
	_, err = dao.CreatePost(ctx, testPost)
	require.NoError(t, err)

	filteredPosts, err := dao.ListPosts(ctx, bson.M{"source_platform": "special_platform"}, 10, 0)
	require.NoError(t, err)
	assert.Len(t, filteredPosts, 1)
}

func TestMongoDAO_UpdatePost(t *testing.T) {
	dao, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	post := FromSocialPost(createTestPost())

	// Create post first
	id, err := dao.CreatePost(ctx, post)
	require.NoError(t, err)

	// Get the created post
	retrievedPost, err := dao.GetPostByID(ctx, id)
	require.NoError(t, err)

	// Update post
	updatedContent := "Updated content"
	retrievedPost.Content = updatedContent
	err = dao.UpdatePost(ctx, retrievedPost)
	require.NoError(t, err)

	// Verify update
	updatedRetrievedPost, err := dao.GetPostByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, updatedContent, updatedRetrievedPost.Content)
}

func TestMongoDAO_DeletePost(t *testing.T) {
	dao, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	post := FromSocialPost(createTestPost())

	// Create post first
	id, err := dao.CreatePost(ctx, post)
	require.NoError(t, err)

	// Delete the post
	err = dao.DeletePost(ctx, id)
	require.NoError(t, err)

	// Verify it's deleted
	deletedPost, err := dao.GetPostByID(ctx, id)
	require.NoError(t, err)
	assert.Nil(t, deletedPost)
}

func TestMongoDAO_UpdateCrossPostStatus(t *testing.T) {
	dao, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	post := FromSocialPost(createTestPost())

	// Create post first
	id, err := dao.CreatePost(ctx, post)
	require.NoError(t, err)

	// Update cross post status
	platform := "twitter"
	status := CrossPostStatus{
		Success:     true,
		PlatformID:  "twitter_post_id",
		CrossPosted: true,
		PostedAt:    &time.Time{},
	}

	err = dao.UpdateCrossPostStatus(ctx, id, platform, status)
	require.NoError(t, err)

	// Verify the update
	updatedPost, err := dao.GetPostByID(ctx, id)
	require.NoError(t, err)
	assert.NotNil(t, updatedPost.CrossPostStatus[platform])
	assert.True(t, updatedPost.CrossPostStatus[platform].Success)
	assert.Equal(t, "twitter_post_id", updatedPost.CrossPostStatus[platform].PlatformID)
}
