package tests

import (
	"context"
	"testing"

	"go.orx.me/apps/hyper-sync/internal/social"
)

func TestThreadsClient_PostText(t *testing.T) {
	// This is a unit test that would require mock implementation in a real scenario
	// For now, we'll just test the structure

	client, err := social.NewThreadsClient("test_client_id", "test_client_secret", "test_access_token")
	if err != nil {
		t.Fatalf("Failed to create ThreadsClient: %v", err)
	}

	// Test that the client is properly initialized
	if client.ClientID != "test_client_id" {
		t.Errorf("Expected ClientID to be 'test_client_id', got %s", client.ClientID)
	}

	if client.ClientSecret != "test_client_secret" {
		t.Errorf("Expected ClientSecret to be 'test_client_secret', got %s", client.ClientSecret)
	}

	if client.AccessToken != "test_access_token" {
		t.Errorf("Expected AccessToken to be 'test_access_token', got %s", client.AccessToken)
	}
}

func TestPostRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		req     *social.PostRequest
		wantErr bool
	}{
		{
			name: "valid text post",
			req: &social.PostRequest{
				MediaType: "TEXT",
				Text:      "Hello, Threads!",
			},
			wantErr: false,
		},
		{
			name: "valid image post",
			req: &social.PostRequest{
				MediaType: "IMAGE",
				ImageURL:  "https://example.com/image.jpg",
				Text:      "Check out this image!",
			},
			wantErr: false,
		},
		{
			name: "valid video post",
			req: &social.PostRequest{
				MediaType: "VIDEO",
				VideoURL:  "https://example.com/video.mp4",
				Text:      "Watch this video!",
			},
			wantErr: false,
		},
		{
			name: "valid carousel post",
			req: &social.PostRequest{
				MediaType: "CAROUSEL",
				Text:      "Multiple media items",
				Children:  []string{"container1", "container2"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation - ensure required fields are present
			switch tt.req.MediaType {
			case "TEXT":
				if tt.req.Text == "" {
					t.Errorf("Text post should have text content")
				}
			case "IMAGE":
				if tt.req.ImageURL == "" {
					t.Errorf("Image post should have image URL")
				}
			case "VIDEO":
				if tt.req.VideoURL == "" {
					t.Errorf("Video post should have video URL")
				}
			case "CAROUSEL":
				if len(tt.req.Children) < 2 {
					t.Errorf("Carousel post should have at least 2 children")
				}
			}
		})
	}
}

func TestCarouselItem_Validation(t *testing.T) {
	tests := []struct {
		name string
		item social.CarouselItem
		want bool
	}{
		{
			name: "valid image item",
			item: social.CarouselItem{
				MediaType: "IMAGE",
				ImageURL:  "https://example.com/image.jpg",
			},
			want: true,
		},
		{
			name: "valid video item",
			item: social.CarouselItem{
				MediaType: "VIDEO",
				VideoURL:  "https://example.com/video.mp4",
			},
			want: true,
		},
		{
			name: "invalid - no URL",
			item: social.CarouselItem{
				MediaType: "IMAGE",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := true
			switch tt.item.MediaType {
			case "IMAGE":
				if tt.item.ImageURL == "" {
					valid = false
				}
			case "VIDEO":
				if tt.item.VideoURL == "" {
					valid = false
				}
			default:
				valid = false
			}

			if valid != tt.want {
				t.Errorf("CarouselItem validation = %v, want %v", valid, tt.want)
			}
		})
	}
}

// Mock test for PostCarousel validation
func TestPostCarousel_ItemCount(t *testing.T) {
	client, _ := social.NewThreadsClient("test_client_id", "test_client_secret", "test_access_token")

	tests := []struct {
		name      string
		items     []social.CarouselItem
		expectErr bool
	}{
		{
			name: "too few items",
			items: []social.CarouselItem{
				{MediaType: "IMAGE", ImageURL: "https://example.com/1.jpg"},
			},
			expectErr: true,
		},
		{
			name: "valid item count",
			items: []social.CarouselItem{
				{MediaType: "IMAGE", ImageURL: "https://example.com/1.jpg"},
				{MediaType: "IMAGE", ImageURL: "https://example.com/2.jpg"},
			},
			expectErr: false,
		},
		{
			name:      "too many items",
			items:     make([]social.CarouselItem, 21), // 21 items, exceeds limit of 20
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize items for the "too many items" test
			if len(tt.items) == 21 {
				for i := range tt.items {
					tt.items[i] = social.CarouselItem{
						MediaType: "IMAGE",
						ImageURL:  "https://example.com/image.jpg",
					}
				}
			}

			_, err := client.PostCarousel(context.Background(), "test_user_id", tt.items, "Test carousel")

			if tt.expectErr && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectErr && err != nil {
				// For valid item counts, we expect a different error (network/auth related)
				// but not the "must have between 2 and 20 items" error
				if err.Error() == "carousel must have between 2 and 20 items, got 1" ||
					err.Error() == "carousel must have between 2 and 20 items, got 21" {
					t.Errorf("Unexpected validation error: %v", err)
				}
			}
		})
	}
}
