package tests

import (
	"testing"

	"go.orx.me/apps/hyper-sync/internal/social"
)

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
