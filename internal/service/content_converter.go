package service

import (
	"fmt"
	"strings"
	"time"

	"go.orx.me/apps/hyper-sync/internal/social"
)

// ContentConverter handles conversion between different content formats
type ContentConverter struct{}

// NewContentConverter creates a new content converter
func NewContentConverter() *ContentConverter {
	return &ContentConverter{}
}

// MemoToPost converts a Memos memo to a standard Post
func (c *ContentConverter) MemoToPost(memo *social.Memo) (*social.Post, error) {
	if memo == nil {
		return nil, fmt.Errorf("memo cannot be nil")
	}

	// Convert memo content to post content
	content := c.convertMemoContent(memo.Content)

	// Convert visibility to enum
	visibilityStr := c.convertVisibility(memo.Visibility)
	visibility, err := social.ParseVisibilityLevel(visibilityStr)
	if err != nil {
		// Use default visibility if parsing fails
		visibility = social.VisibilityLevelPublic
	}

	// Handle media resources
	media, err := c.convertMemoResources(memo.Resources)
	if err != nil {
		return nil, fmt.Errorf("failed to convert memo resources: %w", err)
	}

	// Create the post
	post := &social.Post{
		Content:        content,
		Visibility:     visibility,
		Media:          media,
		SourcePlatform: "memos",
		OriginalID:     extractMemoID(memo.Name), // Extract ID from "memos/123" format
	}

	return post, nil
}

// convertMemoContent processes memo content for social media posting
func (c *ContentConverter) convertMemoContent(content string) string {
	// Clean up the content
	cleaned := strings.TrimSpace(content)

	// TODO: Add more content processing as needed:
	// - Convert markdown links to plain text
	// - Handle hashtags
	// - Handle mentions
	// - Limit content length per platform

	return cleaned
}

// convertVisibility maps memo visibility to social media visibility
func (c *ContentConverter) convertVisibility(memoVisibility string) string {
	switch strings.ToUpper(memoVisibility) {
	case "PUBLIC":
		return "public"
	case "PROTECTED":
		return "unlisted" // or "followers" depending on platform
	case "PRIVATE":
		return "private"
	default:
		return "public" // Default to public for unknown visibility
	}
}

// convertMemoResources converts memo resources (attachments) to media objects
func (c *ContentConverter) convertMemoResources(resources []social.Resource) ([]social.Media, error) {
	if len(resources) == 0 {
		return nil, nil
	}

	var media []social.Media

	for _, resource := range resources {
		// Only process supported media types
		if !c.isSupportedMediaType(resource.Type) {
			continue
		}

		var mediaObj *social.Media

		// If resource has external link, create media from URL
		if resource.ExternalLink != "" {
			mediaObj = social.NewMediaFromURL(resource.ExternalLink)
		} else if resource.Content != "" {
			// If resource has base64 content, decode it
			// This would need base64 decoding implementation
			// For now, skip content-based resources
			continue
		} else {
			// Skip resources without usable content
			continue
		}

		// Set description if available
		if resource.Filename != "" {
			mediaObj.Description = resource.Filename
		}

		media = append(media, *mediaObj)
	}

	return media, nil
}

// isSupportedMediaType checks if the media type is supported for social media posting
func (c *ContentConverter) isSupportedMediaType(mediaType string) bool {
	supportedTypes := map[string]bool{
		"image/jpeg": true,
		"image/jpg":  true,
		"image/png":  true,
		"image/gif":  true,
		"image/webp": true,
	}

	return supportedTypes[strings.ToLower(mediaType)]
}

// extractMemoID extracts the numeric ID from memo name format "memos/123"
func extractMemoID(memoName string) string {
	parts := strings.Split(memoName, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-1] // Return the last part (the ID)
	}
	return memoName // Return as-is if format is unexpected
}

// PostToMemo converts a Post back to Memo format (if needed)
func (c *ContentConverter) PostToMemo(post *social.Post) (*social.Memo, error) {
	if post == nil {
		return nil, fmt.Errorf("post cannot be nil")
	}

	// This is mainly for reverse conversion if needed
	memo := &social.Memo{
		Content:    post.Content,
		Visibility: c.reverseConvertVisibility(post.Visibility.String()),
		CreateTime: time.Now(),
		UpdateTime: time.Now(),
	}

	return memo, nil
}

// reverseConvertVisibility maps social media visibility back to memo visibility
func (c *ContentConverter) reverseConvertVisibility(socialVisibility string) string {
	switch strings.ToLower(socialVisibility) {
	case "public":
		return "PUBLIC"
	case "unlisted", "followers":
		return "PROTECTED"
	case "private":
		return "PRIVATE"
	default:
		return "PUBLIC"
	}
}
