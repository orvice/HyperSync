package dao

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type MongoDAO struct {
	Client   *mongo.Client
	Database string
}

func NewMongoClient() (*mongo.Client, error) {
	return nil, nil
}

func NewMongoDAO(client *mongo.Client) *MongoDAO {
	return &MongoDAO{
		Client: client,
	}
}

// collection returns the MongoDB collection for users.
func (dao *MongoDAO) collection(name string) *mongo.Collection {
	return dao.Client.Database(dao.Database).Collection(name)
}

// CreateUser inserts a new user into the database.
func (dao *MongoDAO) CreateUser(ctx context.Context, user *User) error {
	user.ID = primitive.NewObjectID()
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	_, err := dao.collection(userCollection).InsertOne(ctx, user)
	if mongo.IsDuplicateKeyError(err) {
		return ErrUserAlreadyExists
	}
	return err
}

// GetUserByID retrieves a user by their MongoDB ObjectID.
func (dao *MongoDAO) GetUserByID(ctx context.Context, id primitive.ObjectID) (*User, error) {
	var user User
	err := dao.collection(userCollection).FindOne(ctx, bson.M{"_id": id}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return nil, ErrUserNotFound
	}
	return &user, err
}

// GetUserByEmail retrieves a user by their email address.
func (dao *MongoDAO) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	err := dao.collection(userCollection).FindOne(ctx, bson.M{"email": email}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return nil, ErrUserNotFound
	}
	return &user, err
}

// GetUserByUsername retrieves a user by their username.
func (dao *MongoDAO) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	err := dao.collection(userCollection).FindOne(ctx, bson.M{"username": username}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return nil, ErrUserNotFound
	}
	return &user, err
}

// GetUserByLogin retrieves a user by either username or email.
func (dao *MongoDAO) GetUserByLogin(ctx context.Context, login string) (*User, error) {
	var user User
	filter := bson.M{
		"$or": []bson.M{
			{"username": login},
			{"email": login},
		},
	}
	err := dao.collection(userCollection).FindOne(ctx, filter).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return nil, ErrUserNotFound
	}
	return &user, err
}

// GetUserByGoogleID retrieves a user by their Google ID.
func (dao *MongoDAO) GetUserByGoogleID(ctx context.Context, googleID string) (*User, error) {
	var user User
	err := dao.collection(userCollection).FindOne(ctx, bson.M{"google_id": googleID}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return nil, ErrUserNotFound
	}
	return &user, err
}

// UpdateUser updates an existing user's information.
func (dao *MongoDAO) UpdateUser(ctx context.Context, user *User) error {
	user.UpdatedAt = time.Now()
	filter := bson.M{"_id": user.ID}
	update := bson.M{
		"$set": bson.M{
			"nickname":      user.Nickname,
			"avatar_url":    user.AvatarURL,
			"password_hash": user.PasswordHash,
			"google_id":     user.GoogleID,
			"updated_at":    user.UpdatedAt,
		},
	}

	result, err := dao.collection(userCollection).UpdateOne(ctx, filter, update)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrUserAlreadyExists
		}
		return err
	}
	if result.MatchedCount == 0 {
		return ErrUserNotFound
	}
	return nil
}
