package media

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const mediaCollection = "media"

type MongoStore struct {
	client   *mongo.Client
	database string
}

func NewMongoStore(client *mongo.Client, database string) *MongoStore {
	return &MongoStore{client: client, database: database}
}

func (s *MongoStore) col() *mongo.Collection {
	return s.client.Database(s.database).Collection(mediaCollection)
}

func (s *MongoStore) Create(ctx context.Context, m *Media) (*Media, error) {
	doc := mediaDocument{
		ID:               bson.NewObjectID(),
		S3Key:            m.S3Key,
		CDNUrl:           m.CDNUrl,
		ContentType:      m.ContentType,
		SizeBytes:        m.SizeBytes,
		OriginalFilename: m.OriginalFilename,
		CreatedAt:        time.Now(),
	}

	_, err := s.col().InsertOne(ctx, doc)
	if err != nil {
		return nil, err
	}

	return docToMedia(&doc), nil
}

func (s *MongoStore) GetByID(ctx context.Context, id string) (*Media, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, ErrNotFound
	}

	var doc mediaDocument
	err = s.col().FindOne(ctx, bson.M{"_id": oid}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return docToMedia(&doc), nil
}

func (s *MongoStore) List(ctx context.Context, opts ListOptions) (*ListResult, error) {
	filter := bson.M{}

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

	var docs []mediaDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	items := make([]*Media, 0, len(docs))
	for i := range docs {
		items = append(items, docToMedia(&docs[i]))
	}

	return &ListResult{Items: items, Total: int(total)}, nil
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

type mediaDocument struct {
	ID               bson.ObjectID `bson:"_id,omitempty"`
	S3Key            string        `bson:"s3_key"`
	CDNUrl           string        `bson:"cdn_url"`
	ContentType      string        `bson:"content_type"`
	SizeBytes        int64         `bson:"size_bytes"`
	OriginalFilename string        `bson:"original_filename"`
	CreatedAt        time.Time     `bson:"created_at"`
}

func docToMedia(doc *mediaDocument) *Media {
	return &Media{
		ID:               doc.ID.Hex(),
		S3Key:            doc.S3Key,
		CDNUrl:           doc.CDNUrl,
		ContentType:      doc.ContentType,
		SizeBytes:        doc.SizeBytes,
		OriginalFilename: doc.OriginalFilename,
		CreatedAt:        doc.CreatedAt,
	}
}
