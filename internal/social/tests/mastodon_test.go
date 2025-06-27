package tests

import (
	"context"
	"os"
	"testing"

	"github.com/mattn/go-mastodon"
	"go.orx.me/apps/hyper-sync/internal/social"
)

func TestNewMastodonClient(t *testing.T) {
	// Skip if environment variables are not set
	if os.Getenv("MASTODON_INSTANCE") == "" || os.Getenv("MASTODON_TOKEN") == "" {
		t.Skip("Skipping test because MASTODON_INSTANCE or MASTODON_TOKEN environment variables are not set")
	}

	instanceURL := os.Getenv("MASTODON_INSTANCE")
	accessToken := os.Getenv("MASTODON_TOKEN")

	client := social.NewMastodonClient(instanceURL, accessToken, "mastodon")

	if client == nil {
		t.Fatalf("Client is nil")
	}

	if client.Client == nil {
		t.Fatalf("Mastodon client is nil")
	}
}

func TestMastodonPost(t *testing.T) {
	// Skip if environment variables are not set
	if os.Getenv("MASTODON_INSTANCE") == "" || os.Getenv("MASTODON_TOKEN") == "" {
		t.Skip("Skipping test because MASTODON_INSTANCE or MASTODON_TOKEN environment variables are not set")
	}

	instanceURL := os.Getenv("MASTODON_INSTANCE")
	accessToken := os.Getenv("MASTODON_TOKEN")

	client := social.NewMastodonClient(instanceURL, accessToken, "mastodon")

	// Test posting - only run this if specifically enabled with TEST_POST=1
	if os.Getenv("TEST_POST") == "1" {
		// Create a post without media
		post := &social.Post{
			Content:    "Test post from HyperSync unit test",
			Visibility: social.VisibilityLevelPublic,
		}

		resp, err := client.Post(context.Background(), post)
		if err != nil {
			t.Fatalf("Error posting to Mastodon: %v", err)
		}

		// Check response
		status, ok := resp.(*mastodon.Status)
		if !ok {
			t.Logf("Response is not a *mastodon.Status: %T", resp)
		} else {
			t.Logf("Successfully posted to Mastodon with ID: %s", status.ID)
		}

		// We don't delete the post here as Mastodon doesn't have a straightforward deletion API
		// that we've implemented in our client
	} else {
		t.Log("Skipping actual post test. Set TEST_POST=1 to enable.")
	}
}

func TestMastodonPostWithMedia(t *testing.T) {
	// Skip if environment variables are not set
	if os.Getenv("MASTODON_INSTANCE") == "" || os.Getenv("MASTODON_TOKEN") == "" {
		t.Skip("Skipping test because MASTODON_INSTANCE or MASTODON_TOKEN environment variables are not set")
	}

	// Skip if not explicitly enabled
	if os.Getenv("TEST_POST") != "1" {
		t.Skip("Skipping actual post test. Set TEST_POST=1 to enable.")
	}

	instanceURL := os.Getenv("MASTODON_INSTANCE")
	accessToken := os.Getenv("MASTODON_TOKEN")

	client := social.NewMastodonClient(instanceURL, accessToken, "mastodon")

	// Create a test image (1x1 pixel PNG)
	// This is a minimal valid PNG file (1x1 transparent pixel)
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89, 0x00, 0x00, 0x00,
		0x0A, 0x49, 0x44, 0x41, 0x54, 0x08, 0xD7, 0x63, 0x60, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x01, 0xE2, 0x21, 0xBC, 0x33, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
	}

	// Create a media object
	media := social.NewMedia(pngData)

	// Create a post with media
	post := &social.Post{
		Content:    "Test post with media from HyperSync unit test",
		Visibility: social.VisibilityLevelPublic,
		Media:      []social.Media{*media},
	}

	// Post with media
	resp, err := client.Post(context.Background(), post)
	if err != nil {
		t.Fatalf("Error posting to Mastodon with media: %v", err)
	}

	// Check response
	status, ok := resp.(*mastodon.Status)
	if !ok {
		t.Logf("Response is not a *mastodon.Status: %T", resp)
	} else {
		t.Logf("Successfully posted to Mastodon with media, ID: %s", status.ID)
	}
}

func TestMastodonPostWithURLMedia(t *testing.T) {
	// Skip if environment variables are not set
	if os.Getenv("MASTODON_INSTANCE") == "" || os.Getenv("MASTODON_TOKEN") == "" {
		t.Skip("Skipping test because MASTODON_INSTANCE or MASTODON_TOKEN environment variables are not set")
	}

	// Skip if not explicitly enabled
	if os.Getenv("TEST_POST") != "1" {
		t.Skip("Skipping actual post test. Set TEST_POST=1 to enable.")
	}

	instanceURL := os.Getenv("MASTODON_INSTANCE")
	accessToken := os.Getenv("MASTODON_TOKEN")

	client := social.NewMastodonClient(instanceURL, accessToken, "mastodon")

	// Create a media object from URL - using a reliable test image
	media := social.NewMediaFromURL("https://httpbin.org/image/png")

	// Create a post with URL-based media
	post := &social.Post{
		Content:    "Test post with URL media from HyperSync unit test",
		Visibility: social.VisibilityLevelPublic,
		Media:      []social.Media{*media},
	}

	// Post with media
	resp, err := client.Post(context.Background(), post)
	if err != nil {
		t.Fatalf("Error posting to Mastodon with URL media: %v", err)
	}

	// Check response
	status, ok := resp.(*mastodon.Status)
	if !ok {
		t.Logf("Response is not a *mastodon.Status: %T", resp)
	} else {
		t.Logf("Successfully posted to Mastodon with URL media, ID: %s", status.ID)
	}
}

func TestMastodonListPosts(t *testing.T) {
	// Skip if environment variables are not set
	if os.Getenv("MASTODON_INSTANCE") == "" || os.Getenv("MASTODON_TOKEN") == "" {
		t.Skip("Skipping test because MASTODON_INSTANCE or MASTODON_TOKEN environment variables are not set")
	}

	instanceURL := os.Getenv("MASTODON_INSTANCE")
	accessToken := os.Getenv("MASTODON_TOKEN")

	client := social.NewMastodonClient(instanceURL, accessToken, "mastodon")

	// Get posts for the authenticated user
	posts, err := client.ListPosts(context.Background(), 5)
	if err != nil {
		t.Fatalf("Error listing posts from Mastodon: %v", err)
	}

	// Log the number of posts retrieved
	t.Logf("Retrieved %d posts from Mastodon", len(posts))

	// Verify post content
	for i, post := range posts {
		if post.ID == "" {
			t.Errorf("Post %d has empty ID", i)
		}

		t.Logf("Post %d: ID=%s, Content=%s", i, post.ID, post.Content)

		// Check if post has media
		if len(post.Media) > 0 {
			t.Logf("Post %d has %d media attachments", i, len(post.Media))
		}
	}
}
