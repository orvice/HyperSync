package post

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// The legacy sync DAO owns "posts" (with a unique index on social/social_id
// that would reject managed posts), so managed posts get their own collection.
const collection = "managed_posts"

type MongoStore struct {
	client   *mongo.Client
	database string
}

func NewMongoStore(client *mongo.Client, database string) *MongoStore {
	return &MongoStore{client: client, database: database}
}

func (s *MongoStore) col() *mongo.Collection {
	return s.client.Database(s.database).Collection(collection)
}

func (s *MongoStore) Create(ctx context.Context, p *Post) (*Post, error) {
	doc := toDocument(p)
	doc.ID = bson.NewObjectID()
	now := time.Now()
	doc.CreatedAt = now
	doc.UpdatedAt = now

	_, err := s.col().InsertOne(ctx, doc)
	if err != nil {
		return nil, err
	}

	return fromDocument(doc), nil
}

func (s *MongoStore) GetByID(ctx context.Context, id string) (*Post, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, ErrNotFound
	}

	var doc postDocument
	err = s.col().FindOne(ctx, bson.M{"_id": oid}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return fromDocument(&doc), nil
}

func (s *MongoStore) List(ctx context.Context, opts ListOptions) (*ListResult, error) {
	filter := bson.M{}
	if opts.Status != "" {
		filter["status"] = opts.Status
	}

	total, err := s.col().CountDocuments(ctx, filter)
	if err != nil {
		return nil, err
	}

	pageSize := int64(opts.PageSize)
	if pageSize <= 0 {
		pageSize = 20
	}
	page := int64(opts.Page)
	if page <= 0 {
		page = 1
	}
	skip := (page - 1) * pageSize

	findOpts := options.Find().
		SetLimit(pageSize).
		SetSkip(skip).
		SetSort(bson.D{{Key: "created_at", Value: -1}})

	cursor, err := s.col().Find(ctx, filter, findOpts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []postDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	posts := make([]*Post, 0, len(docs))
	for i := range docs {
		posts = append(posts, fromDocument(&docs[i]))
	}

	return &ListResult{Posts: posts, Total: int(total)}, nil
}

func (s *MongoStore) Update(ctx context.Context, p *Post) (*Post, error) {
	oid, err := bson.ObjectIDFromHex(p.ID)
	if err != nil {
		return nil, ErrNotFound
	}

	p.UpdatedAt = time.Now()
	doc := toDocument(p)
	doc.ID = oid

	result, err := s.col().ReplaceOne(ctx, bson.M{"_id": oid}, doc)
	if err != nil {
		return nil, err
	}
	if result.MatchedCount == 0 {
		return nil, ErrNotFound
	}

	return fromDocument(doc), nil
}

func (s *MongoStore) UpdateSyncStatus(ctx context.Context, id, platform string, status CrossPostStatus) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return ErrNotFound
	}

	update := bson.M{"$set": bson.M{
		"cross_post_status." + platform: crossPostStatusDoc{
			Success:     status.Success,
			Error:       status.Error,
			PlatformID:  status.PlatformID,
			PostedAt:    status.PostedAt,
			RetryCount:  status.RetryCount,
			NeedsUpdate: status.NeedsUpdate,
			NeedsDelete: status.NeedsDelete,
		},
	}}

	result, err := s.col().UpdateOne(ctx, bson.M{"_id": oid}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *MongoStore) SetSyncPending(ctx context.Context, id string, pending bool) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return ErrNotFound
	}

	result, err := s.col().UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{"sync_pending": pending}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// EnsureIndexes creates the indexes the worker's pending-sync query relies on.
func (s *MongoStore) EnsureIndexes(ctx context.Context) error {
	_, err := s.col().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "sync_pending", Value: 1}, {Key: "status", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "created_at", Value: -1}}},
	})
	return err
}

func (s *MongoStore) Delete(ctx context.Context, id string) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return ErrNotFound
	}

	result, err := s.col().DeleteOne(ctx, bson.M{"_id": oid})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *MongoStore) ListPendingSync(ctx context.Context) ([]*Post, error) {
	// Only posts flagged sync_pending by the service/worker; bounded batch so a
	// backlog cannot turn every tick into a full-collection scan.
	// sync_pending is the canonical gate — the service sets it when there is
	// work (new targets, content changes, or NeedsDelete entries). Dropping
	// the former sync_targets filter allows posts whose only remaining work
	// is NeedsDelete cleanup (empty SyncTargets) to be picked up.
	filter := bson.M{
		"status":       "published",
		"sync_pending": true,
	}

	findOpts := options.Find().
		SetSort(bson.D{{Key: "updated_at", Value: 1}}).
		SetLimit(200)

	cursor, err := s.col().Find(ctx, filter, findOpts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []postDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	var posts []*Post
	for i := range docs {
		posts = append(posts, fromDocument(&docs[i]))
	}
	return posts, nil
}

func (s *MongoStore) RemoveSyncStatus(ctx context.Context, id, platform string) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return ErrNotFound
	}

	update := bson.M{"$unset": bson.M{
		"cross_post_status." + platform: "",
	}}

	result, err := s.col().UpdateOne(ctx, bson.M{"_id": oid}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *MongoStore) ListPendingDelete(ctx context.Context) ([]*Post, error) {
	filter := bson.M{
		"status":       "deleting",
		"sync_pending": true,
	}

	findOpts := options.Find().
		SetSort(bson.D{{Key: "updated_at", Value: 1}}).
		SetLimit(200)

	cursor, err := s.col().Find(ctx, filter, findOpts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []postDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	var posts []*Post
	for i := range docs {
		posts = append(posts, fromDocument(&docs[i]))
	}
	return posts, nil
}

type postDocument struct {
	ID              bson.ObjectID                 `bson:"_id,omitempty"`
	Content         string                        `bson:"content"`
	Visibility      string                        `bson:"visibility"`
	Status          string                        `bson:"status"`
	MediaIDs        []string                      `bson:"media_ids,omitempty"`
	SyncTargets     []string                      `bson:"sync_targets,omitempty"`
	CrossPostStatus map[string]crossPostStatusDoc `bson:"cross_post_status,omitempty"`
	SyncPending     bool                          `bson:"sync_pending"`
	CreatedAt       time.Time                     `bson:"created_at"`
	UpdatedAt       time.Time                     `bson:"updated_at"`
}

type crossPostStatusDoc struct {
	Success     bool       `bson:"success"`
	Error       string     `bson:"error,omitempty"`
	PlatformID  string     `bson:"platform_id,omitempty"`
	PostedAt    *time.Time `bson:"posted_at,omitempty"`
	RetryCount  int        `bson:"retry_count"`
	NeedsUpdate bool       `bson:"needs_update"`
	NeedsDelete bool       `bson:"needs_delete"`
}

func toDocument(p *Post) *postDocument {
	doc := &postDocument{
		Content:     p.Content,
		Visibility:  p.Visibility,
		Status:      p.Status,
		MediaIDs:    p.MediaIDs,
		SyncTargets: p.SyncTargets,
		SyncPending: p.SyncPending,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}

	if len(p.CrossPostStatus) > 0 {
		doc.CrossPostStatus = make(map[string]crossPostStatusDoc)
		for k, v := range p.CrossPostStatus {
			doc.CrossPostStatus[k] = crossPostStatusDoc{
				Success:     v.Success,
				Error:       v.Error,
				PlatformID:  v.PlatformID,
				PostedAt:    v.PostedAt,
				RetryCount:  v.RetryCount,
				NeedsUpdate: v.NeedsUpdate,
				NeedsDelete: v.NeedsDelete,
			}
		}
	}

	return doc
}

func fromDocument(doc *postDocument) *Post {
	p := &Post{
		ID:              doc.ID.Hex(),
		Content:         doc.Content,
		Visibility:      doc.Visibility,
		Status:          doc.Status,
		MediaIDs:        doc.MediaIDs,
		SyncTargets:     doc.SyncTargets,
		CrossPostStatus: make(map[string]CrossPostStatus),
		SyncPending:     doc.SyncPending,
		CreatedAt:       doc.CreatedAt,
		UpdatedAt:       doc.UpdatedAt,
	}

	for k, v := range doc.CrossPostStatus {
		p.CrossPostStatus[k] = CrossPostStatus{
			Success:     v.Success,
			Error:       v.Error,
			PlatformID:  v.PlatformID,
			PostedAt:    v.PostedAt,
			RetryCount:  v.RetryCount,
			NeedsUpdate: v.NeedsUpdate,
			NeedsDelete: v.NeedsDelete,
		}
	}

	return p
}
