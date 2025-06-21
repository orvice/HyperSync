package dao

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	syncRecordsCollection = "sync_records"
)

// SyncRecordModel represents a sync record in the database
type SyncRecordModel struct {
	ID             bson.ObjectID `bson:"_id,omitempty"`
	SourcePlatform string        `bson:"source_platform"`      // e.g., "memos"
	SourceID       string        `bson:"source_id"`            // Original ID from source platform
	SourceURL      string        `bson:"source_url,omitempty"` // URL to original content
	PostID         string        `bson:"post_id,omitempty"`    // ID of created post in our system
	Status         string        `bson:"status"`               // "pending", "synced", "failed", "skipped"
	LastSyncAt     time.Time     `bson:"last_sync_at"`
	CreatedAt      time.Time     `bson:"created_at"`
	UpdatedAt      time.Time     `bson:"updated_at"`
	ErrorMessage   string        `bson:"error_message,omitempty"`
	RetryCount     int           `bson:"retry_count"`
	MaxRetries     int           `bson:"max_retries"`
	// Track which platforms this content was synced to
	SyncTargets map[string]SyncTargetStatus `bson:"sync_targets,omitempty"`
	// Metadata about the original content
	ContentHash    string `bson:"content_hash,omitempty"`    // Hash of content to detect changes
	ContentPreview string `bson:"content_preview,omitempty"` // First 100 chars for preview
}

// SyncTargetStatus tracks sync status for each target platform
type SyncTargetStatus struct {
	Status     string     `bson:"status"`                // "pending", "synced", "failed", "skipped"
	PlatformID string     `bson:"platform_id,omitempty"` // ID on target platform
	SyncedAt   *time.Time `bson:"synced_at,omitempty"`
	Error      string     `bson:"error,omitempty"`
	RetryCount int        `bson:"retry_count"`
}

// Sync status constants
const (
	SyncStatusPending = "pending"
	SyncStatusSynced  = "synced"
	SyncStatusFailed  = "failed"
	SyncStatusSkipped = "skipped"
)

// GetSyncRecord retrieves a sync record by ID
func (d *MongoDAO) GetSyncRecord(ctx context.Context, id string) (*SyncRecordModel, error) {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	collection := d.Client.Database(d.Database).Collection(syncRecordsCollection)

	record := &SyncRecordModel{}
	err = collection.FindOne(ctx, bson.M{"_id": objectID}).Decode(record)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return record, nil
}

// GetSyncRecordBySource retrieves a sync record by source platform and ID
func (d *MongoDAO) GetSyncRecordBySource(ctx context.Context, sourcePlatform, sourceID string) (*SyncRecordModel, error) {
	collection := d.Client.Database(d.Database).Collection(syncRecordsCollection)

	record := &SyncRecordModel{}
	err := collection.FindOne(ctx, bson.M{
		"source_platform": sourcePlatform,
		"source_id":       sourceID,
	}).Decode(record)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return record, nil
}

// ListSyncRecords retrieves sync records with optional filtering
func (d *MongoDAO) ListSyncRecords(ctx context.Context, filter bson.M, limit int64, skip int64) ([]*SyncRecordModel, error) {
	collection := d.Client.Database(d.Database).Collection(syncRecordsCollection)

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	if limit > 0 {
		opts.SetLimit(limit)
	}
	if skip > 0 {
		opts.SetSkip(skip)
	}

	if filter == nil {
		filter = bson.M{}
	}

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var records []*SyncRecordModel
	if err := cursor.All(ctx, &records); err != nil {
		return nil, err
	}

	return records, nil
}

// CreateSyncRecord creates a new sync record
func (d *MongoDAO) CreateSyncRecord(ctx context.Context, record *SyncRecordModel) (string, error) {
	collection := d.Client.Database(d.Database).Collection(syncRecordsCollection)

	now := time.Now()
	record.CreatedAt = now
	record.UpdatedAt = now
	record.LastSyncAt = now

	// Set default values
	if record.Status == "" {
		record.Status = SyncStatusPending
	}
	if record.SyncTargets == nil {
		record.SyncTargets = make(map[string]SyncTargetStatus)
	}

	result, err := collection.InsertOne(ctx, record)
	if err != nil {
		return "", err
	}

	return result.InsertedID.(bson.ObjectID).Hex(), nil
}

// UpdateSyncRecord updates an existing sync record
func (d *MongoDAO) UpdateSyncRecord(ctx context.Context, record *SyncRecordModel) error {
	collection := d.Client.Database(d.Database).Collection(syncRecordsCollection)

	record.UpdatedAt = time.Now()

	_, err := collection.ReplaceOne(ctx, bson.M{"_id": record.ID}, record)
	return err
}

// UpdateSyncRecordStatus updates the status of a sync record
func (d *MongoDAO) UpdateSyncRecordStatus(ctx context.Context, recordID, status string, errorMessage string) error {
	objectID, err := bson.ObjectIDFromHex(recordID)
	if err != nil {
		return err
	}

	collection := d.Client.Database(d.Database).Collection(syncRecordsCollection)

	update := bson.M{
		"$set": bson.M{
			"status":       status,
			"updated_at":   time.Now(),
			"last_sync_at": time.Now(),
		},
	}

	if errorMessage != "" {
		update["$set"].(bson.M)["error_message"] = errorMessage
	}

	if status == SyncStatusFailed {
		update["$inc"] = bson.M{"retry_count": 1}
	}

	_, err = collection.UpdateOne(ctx, bson.M{"_id": objectID}, update)
	return err
}

// UpdateSyncTargetStatus updates the sync status for a specific target platform
func (d *MongoDAO) UpdateSyncTargetStatus(ctx context.Context, recordID, targetPlatform string, status SyncTargetStatus) error {
	objectID, err := bson.ObjectIDFromHex(recordID)
	if err != nil {
		return err
	}

	collection := d.Client.Database(d.Database).Collection(syncRecordsCollection)

	update := bson.M{
		"$set": bson.M{
			fmt.Sprintf("sync_targets.%s", targetPlatform): status,
			"updated_at": time.Now(),
		},
	}

	_, err = collection.UpdateOne(ctx, bson.M{"_id": objectID}, update)
	return err
}

// GetPendingSyncRecords retrieves all sync records that are pending or need retry
func (d *MongoDAO) GetPendingSyncRecords(ctx context.Context, maxRetries int) ([]*SyncRecordModel, error) {
	collection := d.Client.Database(d.Database).Collection(syncRecordsCollection)

	filter := bson.M{
		"$or": []bson.M{
			{"status": SyncStatusPending},
			{
				"status":      SyncStatusFailed,
				"retry_count": bson.M{"$lt": maxRetries},
			},
		},
	}

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}) // Oldest first

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var records []*SyncRecordModel
	if err := cursor.All(ctx, &records); err != nil {
		return nil, err
	}

	return records, nil
}

// GetSyncRecordsByStatus retrieves sync records by status
func (d *MongoDAO) GetSyncRecordsByStatus(ctx context.Context, status string, limit int64) ([]*SyncRecordModel, error) {
	collection := d.Client.Database(d.Database).Collection(syncRecordsCollection)

	filter := bson.M{"status": status}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	if limit > 0 {
		opts.SetLimit(limit)
	}

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var records []*SyncRecordModel
	if err := cursor.All(ctx, &records); err != nil {
		return nil, err
	}

	return records, nil
}

// DeleteSyncRecord deletes a sync record by ID
func (d *MongoDAO) DeleteSyncRecord(ctx context.Context, id string) error {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	collection := d.Client.Database(d.Database).Collection(syncRecordsCollection)
	_, err = collection.DeleteOne(ctx, bson.M{"_id": objectID})
	return err
}

// CleanupOldSyncRecords removes sync records older than the specified duration
func (d *MongoDAO) CleanupOldSyncRecords(ctx context.Context, olderThan time.Duration) (int64, error) {
	collection := d.Client.Database(d.Database).Collection(syncRecordsCollection)

	cutoff := time.Now().Add(-olderThan)
	filter := bson.M{
		"created_at": bson.M{"$lt": cutoff},
		"status":     bson.M{"$in": []string{SyncStatusSynced, SyncStatusSkipped}},
	}

	result, err := collection.DeleteMany(ctx, filter)
	if err != nil {
		return 0, err
	}

	return result.DeletedCount, nil
}
