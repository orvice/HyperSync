package social

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"butterfly.orx.me/core/log"
	twitter "github.com/g8rswimmer/go-twitter/v2"
)

var (
	_ SocialClient = (*TwitterClient)(nil)
)

// OAuth1 implements the twitter.Authorizer interface for OAuth 1.0a authentication
type OAuth1 struct {
	ConsumerKey    string
	ConsumerSecret string
	AccessToken    string
	AccessSecret   string
}

// Add adds the OAuth 1.0a authorization to the HTTP request
func (o *OAuth1) Add(req *http.Request) {
	// Note: This is a simplified OAuth 1.0a implementation
	// In production, you should use a proper OAuth 1.0a library or
	// implement the full signature generation as required by Twitter
	req.Header.Add("Authorization", fmt.Sprintf("OAuth oauth_consumer_key=\"%s\", oauth_token=\"%s\"",
		o.ConsumerKey, o.AccessToken))
}

type TwitterClient struct {
	name   string
	config *TwitterConfig
	client *twitter.Client
}

// NewTwitterClient creates a new Twitter client with the provided configuration
func NewTwitterClient(name string, config *TwitterConfig) *TwitterClient {
	// Create OAuth 1.0a authorizer
	auth := &OAuth1{
		ConsumerKey:    config.ConsumerKey,
		ConsumerSecret: config.ConsumerSecret,
		AccessToken:    config.AccessToken,
		AccessSecret:   config.AccessSecret,
	}

	// Create Twitter API v2 client
	twitterClient := &twitter.Client{
		Authorizer: auth,
		Client:     http.DefaultClient,
		Host:       "https://api.twitter.com",
	}

	return &TwitterClient{
		name:   name,
		config: config,
		client: twitterClient,
	}
}

// Name returns the client name
func (c *TwitterClient) Name() string {
	return c.name
}

// Post publishes a new tweet
func (c *TwitterClient) Post(ctx context.Context, post *Post) (interface{}, error) {
	logger := log.FromContext(ctx)

	if c.client == nil {
		return nil, fmt.Errorf("twitter client not initialized")
	}

	logger.Info("twitter post request",
		"content_length", len(post.Content),
		"media_count", len(post.Media),
		"platform", c.name)

	// Create tweet request
	tweetReq := twitter.CreateTweetRequest{
		Text: post.Content,
	}

	// Handle media attachments if present
	if len(post.Media) > 0 {
		logger.Info("twitter post has media attachments",
			"media_count", len(post.Media))

		// Note: For media uploads, you would need to:
		// 1. First upload media using Twitter's media upload endpoints
		// 2. Get media IDs from upload response
		// 3. Attach media IDs to the tweet
		// This is a complex process that requires additional implementation

		logger.Warn("media attachments not yet fully implemented for twitter")
	}

	// Create the tweet
	response, err := c.client.CreateTweet(ctx, tweetReq)
	if err != nil {
		logger.Error("failed to create tweet",
			"error", err,
			"platform", c.name)
		return nil, fmt.Errorf("failed to create tweet: %w", err)
	}

	// Convert response to our format
	result := map[string]interface{}{
		"id":   response.Tweet.ID,
		"text": response.Tweet.Text,
	}

	logger.Info("twitter post created successfully",
		"tweet_id", response.Tweet.ID,
		"platform", c.name)

	return result, nil
}

// ListPosts retrieves the most recent posts for the authenticated user
func (c *TwitterClient) ListPosts(ctx context.Context, limit int) ([]*Post, error) {
	logger := log.FromContext(ctx)

	if c.client == nil {
		return nil, fmt.Errorf("twitter client not initialized")
	}

	// Set default limit if not specified
	if limit <= 0 {
		limit = 20
	}

	logger.Info("twitter posts request",
		"limit", limit,
		"platform", c.name)

	// First, get the authenticated user's information to get their user ID
	userResp, err := c.client.AuthUserLookup(ctx, twitter.UserLookupOpts{
		UserFields: []twitter.UserField{twitter.UserFieldID},
	})
	if err != nil {
		logger.Error("failed to get authenticated user info",
			"error", err,
			"platform", c.name)
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	if userResp.Raw == nil || len(userResp.Raw.Users) == 0 {
		return nil, fmt.Errorf("no user data returned from twitter")
	}

	userID := userResp.Raw.Users[0].ID

	// Get user's timeline
	timelineOpts := twitter.UserTweetTimelineOpts{
		MaxResults: limit,
		TweetFields: []twitter.TweetField{
			twitter.TweetFieldID,
			twitter.TweetFieldText,
			twitter.TweetFieldCreatedAt,
			twitter.TweetFieldAuthorID,
		},
	}

	timelineResp, err := c.client.UserTweetTimeline(ctx, userID, timelineOpts)
	if err != nil {
		logger.Error("failed to get user timeline",
			"error", err,
			"user_id", userID,
			"platform", c.name)
		return nil, fmt.Errorf("failed to get user timeline: %w", err)
	}

	// Convert Twitter tweets to our Post format
	posts := make([]*Post, 0, len(timelineResp.Raw.Tweets))

	for _, tweet := range timelineResp.Raw.Tweets {
		// Parse created_at time
		var createdAt time.Time
		if tweet.CreatedAt != "" {
			if parsed, err := time.Parse(time.RFC3339, tweet.CreatedAt); err == nil {
				createdAt = parsed
			}
		}

		post := &Post{
			ID:             tweet.ID,
			Content:        tweet.Text,
			SourcePlatform: c.name,
			OriginalID:     tweet.ID,
			CreatedAt:      createdAt,
		}

		// Handle media attachments if present
		if tweet.Attachments != nil && len(tweet.Attachments.MediaKeys) > 0 {
			logger.Debug("tweet has media attachments",
				"tweet_id", post.ID,
				"media_keys", tweet.Attachments.MediaKeys)

			// Note: To get full media information, you would need to:
			// 1. Include media expansions in the request
			// 2. Process the included media data
			// For now, we just note that media exists
			post.Media = make([]Media, len(tweet.Attachments.MediaKeys))
			for i, mediaKey := range tweet.Attachments.MediaKeys {
				post.Media[i] = Media{
					Description: fmt.Sprintf("Media: %s", mediaKey),
				}
			}
		}

		posts = append(posts, post)
	}

	logger.Info("retrieved twitter posts successfully",
		"count", len(posts),
		"platform", c.name)

	return posts, nil
}
