package post

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const collection = "posts"

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

	_, err = s.col().ReplaceOne(ctx, bson.M{"_id": oid}, doc)
	if err != nil {
		return nil, err
	}

	return fromDocument(doc), nil
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
	// Find published posts with sync targets that have either:
	// - a platform not yet synced (success=false, retry < max)
	// - a platform that needs an update (needs_update=true)
	filter := bson.M{
		"status":       "published",
		"sync_targets": bson.M{"$exists": true, "$ne": bson.A{}},
	}

	cursor, err := s.col().Find(ctx, filter)
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
	ID              bson.ObjectID                  `bson:"_id,omitempty"`
	Content         string                         `bson:"content"`
	Visibility      string                         `bson:"visibility"`
	Status          string                         `bson:"status"`
	MediaIDs        []string                       `bson:"media_ids,omitempty"`
	SyncTargets     []string                       `bson:"sync_targets,omitempty"`
	CrossPostStatus map[string]crossPostStatusDoc  `bson:"cross_post_status,omitempty"`
	CreatedAt       time.Time                      `bson:"created_at"`
	UpdatedAt       time.Time                      `bson:"updated_at"`
}

type crossPostStatusDoc struct {
	Success     bool       `bson:"success"`
	Error       string     `bson:"error,omitempty"`
	PlatformID  string     `bson:"platform_id,omitempty"`
	PostedAt    *time.Time `bson:"posted_at,omitempty"`
	RetryCount  int        `bson:"retry_count"`
	NeedsUpdate bool       `bson:"needs_update"`
}

func toDocument(p *Post) *postDocument {
	doc := &postDocument{
		Content:     p.Content,
		Visibility:  p.Visibility,
		Status:      p.Status,
		MediaIDs:    p.MediaIDs,
		SyncTargets: p.SyncTargets,
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
		}
	}

	return p
}
