package social

import (
	"context"
	"fmt"

	"github.com/mattn/go-mastodon"
)

type MastodonClient struct {
	name   string
	Client *mastodon.Client
}

func NewMastodonClient(instanceURL, accessToken, name string) *MastodonClient {
	config := &mastodon.Config{
		Server:      instanceURL,
		AccessToken: accessToken,
	}

	// Create the client
	c := mastodon.NewClient(config)

	return &MastodonClient{
		Client: c,
		name:   name,
	}
}

func (c *MastodonClient) Name() string {
	return c.name
}

// Post publishes a new status to Mastodon
func (c *MastodonClient) Post(ctx context.Context, post *Post) (interface{}, error) {
	// Check if visibility level is supported for Mastodon
	if post.Visibility.IsValid() {
		if !IsVisibilityLevelSupported(PlatformMastodon.String(), post.Visibility) {
			// Skip posting if visibility level is not supported
			return nil, nil
		}
	}

	// Convert enum to platform-specific string
	platformVisibility := GetPlatformVisibilityString(PlatformMastodon.String(), post.Visibility)

	toot := &mastodon.Toot{
		Status:     post.Content,
		Visibility: platformVisibility,
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
		// Convert string visibility to enum
		visibility, err := ParseVisibilityLevel(status.Visibility)
		if err != nil {
			// Use default visibility if parsing fails
			visibility = VisibilityLevelPublic
		}

		post := &Post{
			ID:         string(status.ID),
			Content:    status.Content,
			Visibility: visibility,
		}
		// Add media attachments if available
		if len(status.MediaAttachments) > 0 {
			// We don't have the original media data, just note that media exists
			post.Media = []Media{}
			for _, media := range status.MediaAttachments {
				post.Media = append(post.Media, *NewMediaFromURL(media.URL))
			}
		}
		posts = append(posts, post)
	}

	return posts, nil
}
