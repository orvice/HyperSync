package dao

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	userCollection = "users"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user already exists (username or email)")
)

// User represents a user document in the database.
type User struct {
	ID           primitive.ObjectID `bson:"_id,omitempty"`
	Username     string             `bson:"username"`
	Email        string             `bson:"email"`
	PasswordHash string             `bson:"password_hash"` // Store hashed password
	Nickname     string             `bson:"nickname,omitempty"`
	AvatarURL    string             `bson:"avatar_url,omitempty"`
	GoogleID     string             `bson:"google_id,omitempty"` // For Google Login
	CreatedAt    time.Time          `bson:"created_at"`
	UpdatedAt    time.Time          `bson:"updated_at"`
}

// UserDAO defines the interface for user data operations.
type UserDAO interface {
	CreateUser(ctx context.Context, user *User) error
	GetUserByID(ctx context.Context, id primitive.ObjectID) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	GetUserByLogin(ctx context.Context, login string) (*User, error) // Find by username or email
	GetUserByGoogleID(ctx context.Context, googleID string) (*User, error)
	UpdateUser(ctx context.Context, user *User) error
	// Add other methods as needed, e.g., DeleteUser
}

// mongoUserDAO implements UserDAO using MongoDB.
type mongoUserDAO struct {
	*MongoDAO // Embed MongoDAO to access Client and Database
}

// NewUserDAO creates a new UserDAO implementation backed by MongoDB.
// It assumes MongoDAO is already initialized with a valid client and database name.
func NewUserDAO(mongoDAO *MongoDAO) UserDAO {
	// Ensure indexes are created (idempotent operation)
	go func() {
		// Create unique indexes in the background
		coll := mongoDAO.collection(userCollection)
		_, _ = coll.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
			{Keys: bson.D{{Key: "username", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "google_id", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)}, // Sparse for optional field
		})
	}()
	return &mongoUserDAO{MongoDAO: mongoDAO}
}

// CreateUser inserts a new user into the database.
func (dao *mongoUserDAO) CreateUser(ctx context.Context, user *User) error {
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
func (dao *mongoUserDAO) GetUserByID(ctx context.Context, id primitive.ObjectID) (*User, error) {
	var user User
	err := dao.collection(userCollection).FindOne(ctx, bson.M{"_id": id}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return nil, ErrUserNotFound
	}
	return &user, err
}

// GetUserByEmail retrieves a user by their email address.
func (dao *mongoUserDAO) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	err := dao.collection(userCollection).FindOne(ctx, bson.M{"email": email}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return nil, ErrUserNotFound
	}
	return &user, err
}

// GetUserByUsername retrieves a user by their username.
func (dao *mongoUserDAO) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	err := dao.collection(userCollection).FindOne(ctx, bson.M{"username": username}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return nil, ErrUserNotFound
	}
	return &user, err
}

// GetUserByLogin retrieves a user by either username or email.
func (dao *mongoUserDAO) GetUserByLogin(ctx context.Context, login string) (*User, error) {
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
func (dao *mongoUserDAO) GetUserByGoogleID(ctx context.Context, googleID string) (*User, error) {
	var user User
	err := dao.collection(userCollection).FindOne(ctx, bson.M{"google_id": googleID}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return nil, ErrUserNotFound
	}
	return &user, err
}

// UpdateUser updates an existing user's information.
// It assumes user.ID is set correctly.
func (dao *mongoUserDAO) UpdateUser(ctx context.Context, user *User) error {
	user.UpdatedAt = time.Now()
	filter := bson.M{"_id": user.ID}
	// Use bson.M for flexibility in updates, only setting fields that are provided
	// A more robust approach might involve checking which fields are non-zero/non-nil
	// or using a dedicated update struct.
	update := bson.M{
		"$set": bson.M{
			"nickname":      user.Nickname,
			"avatar_url":    user.AvatarURL,
			"password_hash": user.PasswordHash, // Allow password update if needed
			"google_id":     user.GoogleID,     // Allow linking Google ID
			"updated_at":    user.UpdatedAt,
		},
	}

	result, err := dao.collection(userCollection).UpdateOne(ctx, filter, update)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) { // Handle potential unique constraint violations on update
			return ErrUserAlreadyExists
		}
		return err
	}
	if result.MatchedCount == 0 {
		return ErrUserNotFound
	}
	return nil
}

// NewUser creates a new User instance with the given username and email.
func NewUser(username, email string) *User {
	now := time.Now()
	return &User{
		ID:        primitive.NewObjectID(),
		Username:  username,
		Email:     email,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
