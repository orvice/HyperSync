package auth

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const usersCollection = "users"

type MongoUserStore struct {
	client   *mongo.Client
	database string
}

func NewMongoUserStore(client *mongo.Client, database string) *MongoUserStore {
	return &MongoUserStore{
		client:   client,
		database: database,
	}
}

func (s *MongoUserStore) collection() *mongo.Collection {
	return s.client.Database(s.database).Collection(usersCollection)
}

func (s *MongoUserStore) GetByUsername(ctx context.Context, username string) (*User, error) {
	var doc userDocument
	err := s.collection().FindOne(ctx, bson.M{"username": username}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &User{
		ID:           doc.ID.Hex(),
		Username:     doc.Username,
		PasswordHash: doc.PasswordHash,
	}, nil
}

func (s *MongoUserStore) Create(ctx context.Context, user *User) error {
	doc := userDocument{
		ID:           bson.NewObjectID(),
		Username:     user.Username,
		PasswordHash: user.PasswordHash,
	}
	_, err := s.collection().InsertOne(ctx, doc)
	return err
}

func (s *MongoUserStore) UpdatePassword(ctx context.Context, username string, newHash string) error {
	_, err := s.collection().UpdateOne(
		ctx,
		bson.M{"username": username},
		bson.M{"$set": bson.M{"password_hash": newHash}},
	)
	return err
}

func (s *MongoUserStore) EnsureIndexes(ctx context.Context) error {
	_, err := s.collection().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "username", Value: 1}},
	})
	return err
}

type userDocument struct {
	ID           bson.ObjectID `bson:"_id,omitempty"`
	Username     string        `bson:"username"`
	PasswordHash string        `bson:"password_hash"`
}
