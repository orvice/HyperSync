package dao

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMongoDAO_SyncCursor_GetOffset_Empty(t *testing.T) {
	dao, cleanup := setupTestDB(t)
	defer cleanup()

	mongoDao := dao.(*MongoDAO)
	ctx := context.Background()

	offset, err := mongoDao.GetOffset(ctx, "telegram")
	require.NoError(t, err)
	assert.Equal(t, int64(0), offset, "empty collection should return 0")
}

func TestMongoDAO_SyncCursor_SaveAndGet(t *testing.T) {
	dao, cleanup := setupTestDB(t)
	defer cleanup()

	mongoDao := dao.(*MongoDAO)
	ctx := context.Background()

	err := mongoDao.SaveOffset(ctx, "telegram", 42)
	require.NoError(t, err)

	offset, err := mongoDao.GetOffset(ctx, "telegram")
	require.NoError(t, err)
	assert.Equal(t, int64(42), offset)
}

func TestMongoDAO_SyncCursor_Upsert(t *testing.T) {
	dao, cleanup := setupTestDB(t)
	defer cleanup()

	mongoDao := dao.(*MongoDAO)
	ctx := context.Background()

	err := mongoDao.SaveOffset(ctx, "telegram", 10)
	require.NoError(t, err)

	err = mongoDao.SaveOffset(ctx, "telegram", 99)
	require.NoError(t, err)

	offset, err := mongoDao.GetOffset(ctx, "telegram")
	require.NoError(t, err)
	assert.Equal(t, int64(99), offset, "second save should overwrite first")
}

func TestMongoDAO_SyncCursor_MultiplePlatforms(t *testing.T) {
	dao, cleanup := setupTestDB(t)
	defer cleanup()

	mongoDao := dao.(*MongoDAO)
	ctx := context.Background()

	err := mongoDao.SaveOffset(ctx, "telegram", 100)
	require.NoError(t, err)

	err = mongoDao.SaveOffset(ctx, "other", 200)
	require.NoError(t, err)

	tgOffset, err := mongoDao.GetOffset(ctx, "telegram")
	require.NoError(t, err)
	assert.Equal(t, int64(100), tgOffset)

	otherOffset, err := mongoDao.GetOffset(ctx, "other")
	require.NoError(t, err)
	assert.Equal(t, int64(200), otherOffset)
}
