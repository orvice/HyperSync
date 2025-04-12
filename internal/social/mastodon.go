package social

import (
	"context"
	"fmt"

	"github.com/mattn/go-mastodon"
)

type MastodonClient struct {
	Client *mastodon.Client
}

func NewMastodonClient(instanceURL, accessToken string) *MastodonClient {
	config := &mastodon.Config{
		Server:      instanceURL,
		AccessToken: accessToken,
	}

	// Create the client
	c := mastodon.NewClient(config)

	return &MastodonClient{
		Client: c,
	}
}

// Post publishes a new status to Mastodon
func (c *MastodonClient) Post(ctx context.Context, post *Post) (interface{}, error) {
	toot := &mastodon.Toot{
		Status:     post.Content,
		Visibility: post.Visibility,
	}

	// Upload media attachments if any
	if len(post.Media) > 0 {
		mediaIDs := make([]mastodon.ID, 0, len(post.Media))
		for _, media := range post.Media {
			// Get media data, which might be fetched from a URL
			mediaData, err := media.GetData()
			if err != nil {
				return nil, fmt.Errorf("failed to get media data: %w", err)
			}

			attachment, err := c.Client.UploadMediaFromBytes(ctx, mediaData)
			if err != nil {
				return nil, err
			}
			mediaIDs = append(mediaIDs, attachment.ID)
		}

		// Add media IDs to the toot
		if len(mediaIDs) > 0 {
			toot.MediaIDs = mediaIDs
		}
	}

	status, err := c.Client.PostStatus(ctx, toot)
	return status, err
}

// ListPosts retrieves the most recent posts for the authenticated user
func (c *MastodonClient) ListPosts(ctx context.Context, limit int) ([]*Post, error) {
	// Get the account information for the authenticated user
	account, err := c.Client.GetAccountCurrentUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current user account: %w", err)
	}

	// Set default limit if not specified
	if limit <= 0 {
		limit = 20
	}

	// Get statuses for the authenticated user
	pg := mastodon.Pagination{
		Limit: int64(limit),
	}
	statuses, err := c.Client.GetAccountStatuses(ctx, account.ID, &pg)
	if err != nil {
		return nil, fmt.Errorf("failed to get account statuses: %w", err)
	}

	// Convert Mastodon statuses to our Post type
	posts := make([]*Post, 0, len(statuses))
	for _, status := range statuses {
		post := &Post{
			ID:         string(status.ID),
			Content:    status.Content,
			Visibility: status.Visibility,
		}

		// Add media attachments if available
		if len(status.MediaAttachments) > 0 {
			// We don't have the original media data, just note that media exists
			post.Media = []Media{}
		}

		posts = append(posts, post)
	}

	return posts, nil
}
