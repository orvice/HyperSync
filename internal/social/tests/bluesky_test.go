package tests

import (
	"context"
	"os"
	"strings"
	"testing"

	"go.orx.me/apps/hyper-sync/internal/social"
)

func TestNewBlueskyClientFromEnv(t *testing.T) {
	// Skip if environment variables are not set
	if os.Getenv("BLUESKY_HANDLE") == "" || os.Getenv("BLUESKY_PASSWORD") == "" {
		t.Skip("Skipping test because BLUESKY_HANDLE or BLUESKY_PASSWORD environment variables are not set")
	}

	client, err := social.NewBlueskyClientFromEnv()
	if err != nil {
		t.Fatalf("Error creating Bluesky client: %v", err)
	}

	if client == nil {
		t.Fatalf("Client is nil")
	}

	if client.Client == nil {
		t.Fatalf("XRPC client is nil")
	}

	if client.Client.Auth == nil {
		t.Fatalf("Auth is nil")
	}

	if client.Client.Auth.Did == "" {
		t.Fatalf("DID is empty")
	}
}

func TestPost(t *testing.T) {
	// Skip if environment variables are not set
	if os.Getenv("BLUESKY_HANDLE") == "" || os.Getenv("BLUESKY_PASSWORD") == "" {
		t.Skip("Skipping test because BLUESKY_HANDLE or BLUESKY_PASSWORD environment variables are not set")
	}

	client, err := social.NewBlueskyClientFromEnv()
	if err != nil {
		t.Fatalf("Error creating Bluesky client: %v", err)
	}

	// Test posting - only run this if specifically enabled with TEST_POST=1
	if os.Getenv("TEST_POST") == "1" {
		// Create a post without media
		post := &social.Post{
			Content:    "Test post from HyperSync unit test",
			Visibility: "public",
		}

		resp, err := client.Post(context.Background(), post)
		if err != nil {
			t.Fatalf("Error posting to Bluesky: %v", err)
		}

		// Extract rkey from response
		var rkey string
		switch v := resp.(type) {
		case string:
			rkey = v
		case map[string]interface{}:
			if uri, ok := v["uri"].(string); ok {
				// Extract rkey from URI (at://did:plc:xxx/app.bsky.feed.post/rkey)
				parts := strings.Split(uri, "/")
				if len(parts) > 0 {
					rkey = parts[len(parts)-1]
				}
			}
		default:
			// Try to convert to our anonymous struct
			respData, ok := resp.(struct {
				URI string `json:"uri"`
				CID string `json:"cid"`
			})
			if ok && respData.URI != "" {
				parts := strings.Split(respData.URI, "/")
				if len(parts) > 0 {
					rkey = parts[len(parts)-1]
				}
			} else {
				t.Logf("Unexpected response type: %T", v)
				rkey = "unknown"
			}
		}

		t.Logf("Successfully posted to Bluesky with record key: %s", rkey)

		// Test deletion of the post we just created
		err = client.DeletePost(context.Background(), rkey)
		if err != nil {
			t.Fatalf("Error deleting post from Bluesky: %v", err)
		}

		t.Logf("Successfully deleted post with record key: %s", rkey)
	} else {
		t.Log("Skipping actual post/delete test. Set TEST_POST=1 to enable.")
	}
}

func TestPostWithMedia(t *testing.T) {
	// Skip if environment variables are not set
	if os.Getenv("BLUESKY_HANDLE") == "" || os.Getenv("BLUESKY_PASSWORD") == "" {
		t.Skip("Skipping test because BLUESKY_HANDLE or BLUESKY_PASSWORD environment variables are not set")
	}

	// Skip if not explicitly enabled
	if os.Getenv("TEST_POST") != "1" {
		t.Skip("Skipping actual post/delete test. Set TEST_POST=1 to enable.")
	}

	client, err := social.NewBlueskyClientFromEnv()
	if err != nil {
		t.Fatalf("Error creating Bluesky client: %v", err)
	}

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
		Visibility: "public",
		Media:      []social.Media{*media},
	}

	// Post with media
	resp, err := client.Post(context.Background(), post)
	if err != nil {
		t.Fatalf("Error posting to Bluesky with media: %v", err)
	}

	// Extract rkey from response
	var rkey string
	switch v := resp.(type) {
	case string:
		rkey = v
	case map[string]interface{}:
		if uri, ok := v["uri"].(string); ok {
			// Extract rkey from URI (at://did:plc:xxx/app.bsky.feed.post/rkey)
			parts := strings.Split(uri, "/")
			if len(parts) > 0 {
				rkey = parts[len(parts)-1]
			}
		}
	default:
		// Try to convert to our anonymous struct
		respData, ok := resp.(struct {
			URI string `json:"uri"`
			CID string `json:"cid"`
		})
		if ok && respData.URI != "" {
			parts := strings.Split(respData.URI, "/")
			if len(parts) > 0 {
				rkey = parts[len(parts)-1]
			}
		} else {
			t.Logf("Unexpected response type: %T", v)
			rkey = "unknown"
		}
	}

	t.Logf("Successfully posted to Bluesky with media, record key: %s", rkey)

	// Clean up by deleting the post
	err = client.DeletePost(context.Background(), rkey)
	if err != nil {
		t.Fatalf("Error deleting post with media from Bluesky: %v", err)
	}

	t.Logf("Successfully deleted post with media, record key: %s", rkey)
}

func TestPostWithURLMedia(t *testing.T) {
	// Skip if environment variables are not set
	if os.Getenv("BLUESKY_HANDLE") == "" || os.Getenv("BLUESKY_PASSWORD") == "" {
		t.Skip("Skipping test because BLUESKY_HANDLE or BLUESKY_PASSWORD environment variables are not set")
	}

	// Skip if not explicitly enabled
	if os.Getenv("TEST_POST") != "1" {
		t.Skip("Skipping actual post/delete test. Set TEST_POST=1 to enable.")
	}

	client, err := social.NewBlueskyClientFromEnv()
	if err != nil {
		t.Fatalf("Error creating Bluesky client: %v", err)
	}

	// Create a media object from URL - using a reliable test image
	media := social.NewMediaFromURL("https://httpbin.org/image/png")

	// Create a post with URL-based media
	post := &social.Post{
		Content:    "Test post with URL media from HyperSync unit test",
		Visibility: "public",
		Media:      []social.Media{*media},
	}

	// Post with media
	resp, err := client.Post(context.Background(), post)
	if err != nil {
		t.Fatalf("Error posting to Bluesky with URL media: %v", err)
	}

	// Extract rkey from response
	var rkey string
	switch v := resp.(type) {
	case string:
		rkey = v
	case map[string]interface{}:
		if uri, ok := v["uri"].(string); ok {
			// Extract rkey from URI (at://did:plc:xxx/app.bsky.feed.post/rkey)
			parts := strings.Split(uri, "/")
			if len(parts) > 0 {
				rkey = parts[len(parts)-1]
			}
		}
	default:
		// Try to convert to our anonymous struct
		respData, ok := resp.(struct {
			URI string `json:"uri"`
			CID string `json:"cid"`
		})
		if ok && respData.URI != "" {
			parts := strings.Split(respData.URI, "/")
			if len(parts) > 0 {
				rkey = parts[len(parts)-1]
			}
		} else {
			t.Logf("Unexpected response type: %T", v)
			rkey = "unknown"
		}
	}

	t.Logf("Successfully posted to Bluesky with URL media, record key: %s", rkey)

	// Clean up by deleting the post
	err = client.DeletePost(context.Background(), rkey)
	if err != nil {
		t.Fatalf("Error deleting post with URL media from Bluesky: %v", err)
	}

	t.Logf("Successfully deleted post with URL media, record key: %s", rkey)
}

func TestListPosts(t *testing.T) {
	// Skip if environment variables are not set
	if os.Getenv("BLUESKY_HANDLE") == "" || os.Getenv("BLUESKY_PASSWORD") == "" {
		t.Skip("Skipping test because BLUESKY_HANDLE or BLUESKY_PASSWORD environment variables are not set")
	}

	client, err := social.NewBlueskyClientFromEnv()
	if err != nil {
		t.Fatalf("Error creating Bluesky client: %v", err)
	}

	// Get posts for the authenticated user
	posts, err := client.ListPosts(context.Background(), 5)
	if err != nil {
		t.Fatalf("Error listing posts from Bluesky: %v", err)
	}

	// Log the number of posts retrieved
	t.Logf("Retrieved %d posts from Bluesky", len(posts))

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
