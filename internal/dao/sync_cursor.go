package dao

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"go.orx.me/apps/hyper-sync/internal/social"
)

const syncCursorsCollection = "sync_cursors"

// Compile-time check that MongoDAO satisfies SyncCursorDao.
var _ social.SyncCursorDao = (*MongoDAO)(nil)

type syncCursorModel struct {
	Platform string `bson:"platform"`
	Offset   int64  `bson:"offset"`
}

func (d *MongoDAO) GetOffset(ctx context.Context, platform string) (int64, error) {
	coll := d.Client.Database(d.Database).Collection(syncCursorsCollection)

	var doc syncCursorModel
	err := coll.FindOne(ctx, bson.M{"platform": platform}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return 0, nil
		}
		return 0, err
	}
	return doc.Offset, nil
}

func (d *MongoDAO) SaveOffset(ctx context.Context, platform string, offset int64) error {
	coll := d.Client.Database(d.Database).Collection(syncCursorsCollection)

	filter := bson.M{"platform": platform}
	update := bson.M{"$set": bson.M{"platform": platform, "offset": offset}}
	opts := options.UpdateOne().SetUpsert(true)

	_, err := coll.UpdateOne(ctx, filter, update, opts)
	return err
}
