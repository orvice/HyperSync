package dao

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"go.orx.me/apps/hyper-sync/internal/social"
)

const (
	postsCollection = "posts"
)

// PostDao defines the interface for post data access operations
type PostDao interface {
	// GetPostByID retrieves a post by its ID
	GetPostByID(ctx context.Context, id string) (*PostModel, error)

	// GetPostByOriginalID retrieves a post by its original platform ID
	GetPostByOriginalID(ctx context.Context, platform, originalID string) (*PostModel, error)

	// GetBySocialAndSocialID retrieves a post by social platform and social ID
	GetBySocialAndSocialID(ctx context.Context, social, socialID string) (*PostModel, error)

	// ListPosts retrieves posts with optional filtering
	ListPosts(ctx context.Context, filter map[string]interface{}, limit int64, skip int64) ([]*PostModel, error)

	// CreatePost creates a new post and returns its ID
	CreatePost(ctx context.Context, post *PostModel) (string, error)

	// UpdatePost updates an existing post
	UpdatePost(ctx context.Context, post *PostModel) error

	// DeletePost deletes a post by its ID
	DeletePost(ctx context.Context, id string) error

	// UpdateCrossPostStatus updates the cross-post status for a platform
	UpdateCrossPostStatus(ctx context.Context, postID, platform string, status CrossPostStatus) error
}

// Ensure MongoDAO implements PostDao interface
var _ PostDao = (*MongoDAO)(nil)

// PostModel represents a post in the database
type PostModel struct {
	ID             bson.ObjectID `bson:"_id,omitempty"`
	Social         string        `bson:"social"`
	SocialID       string        `bson:"social_id"`
	Content        string        `bson:"content"`
	Visibility     string        `bson:"visibility"`
	SourcePlatform string        `bson:"source_platform"`
	OriginalID     string        `bson:"original_id"`
	// Store media references instead of full data
	MediaIDs  []string  `bson:"media_ids,omitempty"`
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	// Cross-posting status for each platform
	CrossPostStatus map[string]CrossPostStatus `bson:"cross_post_status,omitempty"`
}

// CrossPostStatus tracks the status of a cross-post to a platform
type CrossPostStatus struct {
	Success     bool       `bson:"success"`
	Error       string     `bson:"error,omitempty"`
	PlatformID  string     `bson:"platform_id,omitempty"` // ID on the target platform
	CrossPosted bool       `bson:"cross_posted"`
	PostedAt    *time.Time `bson:"posted_at,omitempty"`
}

// FromSocialPost converts a social.Post to a PostModel
func FromSocialPost(post *social.Post) *PostModel {
	now := time.Now()
	return &PostModel{
		Content:        post.Content,
		Visibility:     post.Visibility,
		SourcePlatform: post.SourcePlatform,
		OriginalID:     post.OriginalID,
		// Media will be stored separately
		CreatedAt:       now,
		UpdatedAt:       now,
		CrossPostStatus: make(map[string]CrossPostStatus),
	}
}

// ToSocialPost converts a PostModel to a social.Post
func (p *PostModel) ToSocialPost() *social.Post {
	return &social.Post{
		ID:             p.ID.Hex(),
		Content:        p.Content,
		Visibility:     p.Visibility,
		SourcePlatform: p.SourcePlatform,
		OriginalID:     p.OriginalID,
		// Media will need to be loaded separately
	}
}

// GetPostByID retrieves a post by its ID
func (d *MongoDAO) GetPostByID(ctx context.Context, id string) (*PostModel, error) {
	// Convert string ID to ObjectID
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	// Get the posts collection
	collection := d.Client.Database(d.Database).Collection(postsCollection)

	// Find the post
	post := &PostModel{}
	err = collection.FindOne(ctx, bson.M{"_id": objectID}).Decode(post)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // Not found
		}
		return nil, err
	}

	return post, nil
}

// GetPostByOriginalID retrieves a post by its original platform ID
func (d *MongoDAO) GetPostByOriginalID(ctx context.Context, platform, originalID string) (*PostModel, error) {
	// Get the posts collection
	collection := d.Client.Database(d.Database).Collection(postsCollection)

	// Find the post
	post := &PostModel{}
	err := collection.FindOne(ctx, bson.M{
		"source_platform": platform,
		"original_id":     originalID,
	}).Decode(post)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // Not found
		}
		return nil, err
	}

	return post, nil
}

// GetBySocialAndSocialID retrieves a post by social platform and social ID
func (d *MongoDAO) GetBySocialAndSocialID(ctx context.Context, social, socialID string) (*PostModel, error) {
	// Get the posts collection
	collection := d.Client.Database(d.Database).Collection(postsCollection)

	// Find the post
	post := &PostModel{}
	err := collection.FindOne(ctx, bson.M{
		"social":    social,
		"social_id": socialID,
	}).Decode(post)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // Not found
		}
		return nil, err
	}

	return post, nil
}

// ListPosts retrieves posts with optional filtering
func (d *MongoDAO) ListPosts(ctx context.Context, filter map[string]interface{}, limit int64, skip int64) ([]*PostModel, error) {
	// Get the posts collection
	collection := d.Client.Database(d.Database).Collection(postsCollection)

	// Create find options
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}) // Sort by created_at descending
	if limit > 0 {
		opts.SetLimit(limit)
	}
	if skip > 0 {
		opts.SetSkip(skip)
	}

	// Convert generic filter to bson.M for MongoDB
	var bsonFilter bson.M
	if filter == nil {
		bsonFilter = bson.M{}
	} else {
		bsonFilter = bson.M(filter)
	}

	// Find posts
	cursor, err := collection.Find(ctx, bsonFilter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	// Decode posts
	var posts []*PostModel
	if err := cursor.All(ctx, &posts); err != nil {
		return nil, err
	}

	return posts, nil
}

// CreatePost creates a new post
func (d *MongoDAO) CreatePost(ctx context.Context, post *PostModel) (string, error) {
	// Get the posts collection
	collection := d.Client.Database(d.Database).Collection(postsCollection)

	// Set timestamps
	now := time.Now()
	post.CreatedAt = now
	post.UpdatedAt = now

	// Insert the post
	result, err := collection.InsertOne(ctx, post)
	if err != nil {
		return "", err
	}

	// Return the ID
	return result.InsertedID.(bson.ObjectID).Hex(), nil
}

// UpdatePost updates an existing post
func (d *MongoDAO) UpdatePost(ctx context.Context, post *PostModel) error {
	// Get the posts collection
	collection := d.Client.Database(d.Database).Collection(postsCollection)

	// Update timestamp
	post.UpdatedAt = time.Now()

	// Update the post
	_, err := collection.ReplaceOne(ctx, bson.M{"_id": post.ID}, post)
	return err
}

// DeletePost deletes a post by its ID
func (d *MongoDAO) DeletePost(ctx context.Context, id string) error {
	// Convert string ID to ObjectID
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	// Get the posts collection
	collection := d.Client.Database(d.Database).Collection(postsCollection)

	// Delete the post
	_, err = collection.DeleteOne(ctx, bson.M{"_id": objectID})
	return err
}

// UpdateCrossPostStatus updates the cross-post status for a platform
func (d *MongoDAO) UpdateCrossPostStatus(ctx context.Context, postID, platform string, status CrossPostStatus) error {
	// Convert string ID to ObjectID
	objectID, err := bson.ObjectIDFromHex(postID)
	if err != nil {
		return err
	}

	// Get the posts collection
	collection := d.Client.Database(d.Database).Collection(postsCollection)

	// Update the cross-post status
	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": objectID},
		bson.M{
			"$set": bson.M{
				"cross_post_status." + platform: status,
				"updated_at":                    time.Now(),
			},
		},
	)
	return err
}
