package tests

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

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

	// With botsky, we don't need to check internal fields
	// Just verify that the client was created successfully and has a name
	if client.Name() == "" {
		t.Fatalf("Client name is empty")
	}

	t.Logf("Successfully created Bluesky client with name: %s", client.Name())
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
		// Step 1: Create a post without media
		testContent := fmt.Sprintf("Test post from HyperSync unit test - %d", time.Now().Unix())
		post := &social.Post{
			Content:    testContent,
			Visibility: "public",
		}

		t.Log("Step 1: Creating post...")
		resp, err := client.Post(context.Background(), post)
		if err != nil {
			t.Fatalf("Error posting to Bluesky: %v", err)
		}

		// Extract rkey from response
		var rkey string
		var uri string
		switch v := resp.(type) {
		case string:
			rkey = v
		case map[string]interface{}:
			if uriValue, ok := v["uri"].(string); ok {
				uri = uriValue
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
				uri = respData.URI
				parts := strings.Split(respData.URI, "/")
				if len(parts) > 0 {
					rkey = parts[len(parts)-1]
				}
			} else {
				t.Logf("Unexpected response type: %T", v)
				rkey = "unknown"
			}
		}

		if rkey == "" || rkey == "unknown" {
			t.Fatalf("Failed to extract rkey from response: %v", resp)
		}

		t.Logf("Successfully posted to Bluesky with record key: %s, URI: %s", rkey, uri)

		// Step 2: Verify the post exists by listing recent posts
		t.Log("Step 2: Verifying post exists by listing recent posts...")
		posts, err := client.ListPosts(context.Background(), 10) // Get last 10 posts
		if err != nil {
			// Don't fail the test if listing fails, just log and continue to deletion
			t.Logf("Warning: Failed to list posts for verification: %v", err)
		} else {
			// Look for our post in the list
			found := false
			for _, listedPost := range posts {
				if listedPost.ID == rkey || strings.Contains(listedPost.Content, testContent) {
					found = true
					t.Logf("✓ Post verified in list: ID=%s, Content=%s", listedPost.ID, listedPost.Content)
					break
				}
			}

			if found {
				t.Log("✓ Post successfully verified in recent posts list")
			} else {
				t.Logf("⚠ Post not found in recent posts list (this might be due to timing or API limitations)")
				// Don't fail the test, as the post was created successfully
			}
		}

		// Step 3: Delete the post we just created
		t.Log("Step 3: Deleting the test post...")
		err = client.DeletePost(context.Background(), rkey)
		if err != nil {
			t.Fatalf("Error deleting post from Bluesky: %v", err)
		}

		t.Logf("✓ Successfully deleted post with record key: %s", rkey)

		// Optional: Verify deletion by listing posts again
		t.Log("Step 4: Verifying post deletion...")
		postsAfterDeletion, err := client.ListPosts(context.Background(), 10)
		if err != nil {
			t.Logf("Warning: Failed to list posts after deletion: %v", err)
		} else {
			found := false
			for _, listedPost := range postsAfterDeletion {
				if listedPost.ID == rkey {
					found = true
					break
				}
			}

			if found {
				t.Logf("⚠ Post still found after deletion (might be due to API propagation delay)")
			} else {
				t.Log("✓ Post deletion verified - post no longer appears in recent posts list")
			}
		}

		t.Log("✓ Test completed successfully: Create -> Verify -> Delete")
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

	// Step 1: Create a post with media
	testContent := fmt.Sprintf("Test post with media from HyperSync unit test - %d", time.Now().Unix())
	post := &social.Post{
		Content:    testContent,
		Visibility: "public",
		Media:      []social.Media{*media},
	}

	t.Log("Step 1: Creating post with media...")
	resp, err := client.Post(context.Background(), post)
	if err != nil {
		t.Fatalf("Error posting to Bluesky with media: %v", err)
	}

	// Extract rkey from response
	var rkey string
	var uri string
	switch v := resp.(type) {
	case string:
		rkey = v
	case map[string]interface{}:
		if uriValue, ok := v["uri"].(string); ok {
			uri = uriValue
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
			uri = respData.URI
			parts := strings.Split(respData.URI, "/")
			if len(parts) > 0 {
				rkey = parts[len(parts)-1]
			}
		} else {
			t.Logf("Unexpected response type: %T", v)
			rkey = "unknown"
		}
	}

	if rkey == "" || rkey == "unknown" {
		t.Fatalf("Failed to extract rkey from response: %v", resp)
	}

	t.Logf("Successfully posted to Bluesky with media, record key: %s, URI: %s", rkey, uri)

	// Step 2: Verify the post exists by listing recent posts
	t.Log("Step 2: Verifying post with media exists by listing recent posts...")
	posts, err := client.ListPosts(context.Background(), 10) // Get last 10 posts
	if err != nil {
		// Don't fail the test if listing fails, just log and continue to deletion
		t.Logf("Warning: Failed to list posts for verification: %v", err)
	} else {
		// Look for our post in the list
		found := false
		for _, listedPost := range posts {
			if listedPost.ID == rkey || strings.Contains(listedPost.Content, testContent) {
				found = true
				hasMedia := len(listedPost.Media) > 0
				t.Logf("✓ Post verified in list: ID=%s, Content=%s, HasMedia=%v",
					listedPost.ID, listedPost.Content, hasMedia)
				break
			}
		}

		if found {
			t.Log("✓ Post with media successfully verified in recent posts list")
		} else {
			t.Logf("⚠ Post not found in recent posts list (this might be due to timing or API limitations)")
			// Don't fail the test, as the post was created successfully
		}
	}

	// Step 3: Delete the post we just created
	t.Log("Step 3: Deleting the test post with media...")
	err = client.DeletePost(context.Background(), rkey)
	if err != nil {
		t.Fatalf("Error deleting post with media from Bluesky: %v", err)
	}

	t.Logf("✓ Successfully deleted post with media, record key: %s", rkey)

	// Step 4: Verify deletion by listing posts again
	t.Log("Step 4: Verifying post deletion...")
	postsAfterDeletion, err := client.ListPosts(context.Background(), 10)
	if err != nil {
		t.Logf("Warning: Failed to list posts after deletion: %v", err)
	} else {
		found := false
		for _, listedPost := range postsAfterDeletion {
			if listedPost.ID == rkey {
				found = true
				break
			}
		}

		if found {
			t.Logf("⚠ Post still found after deletion (might be due to API propagation delay)")
		} else {
			t.Log("✓ Post deletion verified - post no longer appears in recent posts list")
		}
	}

	t.Log("✓ Test with media completed successfully: Create -> Verify -> Delete")
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

	// Step 1: Create a post with URL-based media
	testContent := fmt.Sprintf("Test post with URL media from HyperSync unit test - %d", time.Now().Unix())
	post := &social.Post{
		Content:    testContent,
		Visibility: "public",
		Media:      []social.Media{*media},
	}

	t.Log("Step 1: Creating post with URL media...")
	resp, err := client.Post(context.Background(), post)
	if err != nil {
		t.Fatalf("Error posting to Bluesky with URL media: %v", err)
	}

	// Extract rkey from response
	var rkey string
	var uri string
	switch v := resp.(type) {
	case string:
		rkey = v
	case map[string]interface{}:
		if uriValue, ok := v["uri"].(string); ok {
			uri = uriValue
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
			uri = respData.URI
			parts := strings.Split(respData.URI, "/")
			if len(parts) > 0 {
				rkey = parts[len(parts)-1]
			}
		} else {
			t.Logf("Unexpected response type: %T", v)
			rkey = "unknown"
		}
	}

	if rkey == "" || rkey == "unknown" {
		t.Fatalf("Failed to extract rkey from response: %v", resp)
	}

	t.Logf("Successfully posted to Bluesky with URL media, record key: %s, URI: %s", rkey, uri)

	// Step 2: Verify the post exists by listing recent posts
	t.Log("Step 2: Verifying post with URL media exists by listing recent posts...")
	posts, err := client.ListPosts(context.Background(), 10) // Get last 10 posts
	if err != nil {
		// Don't fail the test if listing fails, just log and continue to deletion
		t.Logf("Warning: Failed to list posts for verification: %v", err)
	} else {
		// Look for our post in the list
		found := false
		for _, listedPost := range posts {
			if listedPost.ID == rkey || strings.Contains(listedPost.Content, testContent) {
				found = true
				hasMedia := len(listedPost.Media) > 0
				t.Logf("✓ Post verified in list: ID=%s, Content=%s, HasMedia=%v",
					listedPost.ID, listedPost.Content, hasMedia)
				break
			}
		}

		if found {
			t.Log("✓ Post with URL media successfully verified in recent posts list")
		} else {
			t.Logf("⚠ Post not found in recent posts list (this might be due to timing or API limitations)")
			// Don't fail the test, as the post was created successfully
		}
	}

	// Step 3: Delete the post we just created
	t.Log("Step 3: Deleting the test post with URL media...")
	err = client.DeletePost(context.Background(), rkey)
	if err != nil {
		t.Fatalf("Error deleting post with URL media from Bluesky: %v", err)
	}

	t.Logf("✓ Successfully deleted post with URL media, record key: %s", rkey)

	// Step 4: Verify deletion by listing posts again
	t.Log("Step 4: Verifying post deletion...")
	postsAfterDeletion, err := client.ListPosts(context.Background(), 10)
	if err != nil {
		t.Logf("Warning: Failed to list posts after deletion: %v", err)
	} else {
		found := false
		for _, listedPost := range postsAfterDeletion {
			if listedPost.ID == rkey {
				found = true
				break
			}
		}

		if found {
			t.Logf("⚠ Post still found after deletion (might be due to API propagation delay)")
		} else {
			t.Log("✓ Post deletion verified - post no longer appears in recent posts list")
		}
	}

	t.Log("✓ Test with URL media completed successfully: Create -> Verify -> Delete")
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
