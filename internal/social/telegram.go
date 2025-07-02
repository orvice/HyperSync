package social

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"
)

type Telegram struct {
	Token  string
	ChatID string
	name   string
	client *http.Client
}

// TelegramResponse represents the standard Telegram API response
type TelegramResponse struct {
	Ok          bool            `json:"ok"`
	Result      json.RawMessage `json:"result,omitempty"`
	ErrorCode   int             `json:"error_code,omitempty"`
	Description string          `json:"description,omitempty"`
}

// TelegramMessage represents a Telegram message
type TelegramMessage struct {
	MessageID int                 `json:"message_id"`
	From      *TelegramUser       `json:"from,omitempty"`
	Date      int64               `json:"date"`
	Chat      *TelegramChat       `json:"chat"`
	Text      string              `json:"text,omitempty"`
	Caption   string              `json:"caption,omitempty"`
	Photo     []TelegramPhotoSize `json:"photo,omitempty"`
}

// TelegramUser represents a Telegram user
type TelegramUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

// TelegramChat represents a Telegram chat
type TelegramChat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
}

// TelegramPhotoSize represents a photo size
type TelegramPhotoSize struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int    `json:"file_size,omitempty"`
}

// TelegramGetUpdatesResponse represents the response from getUpdates
type TelegramGetUpdatesResponse struct {
	UpdateID int              `json:"update_id"`
	Message  *TelegramMessage `json:"message,omitempty"`
}

func NewTelegram(token, chatID, name string) *Telegram {
	return &Telegram{
		Token:  token,
		ChatID: chatID,
		name:   name,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (t *Telegram) Name() string {
	return t.name
}

// Post sends a message to Telegram
func (t *Telegram) Post(ctx context.Context, post *Post) (interface{}, error) {
	// Check if visibility level is supported for Telegram
	if post.Visibility.IsValid() {
		if !IsVisibilityLevelSupported(PlatformTelegram.String(), post.Visibility) {
			// Skip posting if visibility level is not supported
			return nil, nil
		}
	}

	// If there are media attachments, send as photo/document with caption
	if len(post.Media) > 0 {
		return t.sendMediaMessage(ctx, post)
	}

	// Send as text message
	return t.sendTextMessage(ctx, post.Content)
}

// sendTextMessage sends a text message to Telegram
func (t *Telegram) sendTextMessage(ctx context.Context, text string) (*TelegramMessage, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.Token)

	payload := map[string]interface{}{
		"chat_id": t.ChatID,
		"text":    text,
	}

	return t.makeAPICall(ctx, url, payload)
}

// sendMediaMessage sends a message with media to Telegram
func (t *Telegram) sendMediaMessage(ctx context.Context, post *Post) (*TelegramMessage, error) {
	// For now, we'll send the first media attachment as a photo with caption
	// TODO: Support multiple media attachments
	if len(post.Media) == 0 {
		return t.sendTextMessage(ctx, post.Content)
	}

	media := post.Media[0]
	mediaData, err := media.GetData()
	if err != nil {
		return nil, fmt.Errorf("failed to get media data: %w", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendPhoto", t.Token)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add chat_id field
	if err := writer.WriteField("chat_id", t.ChatID); err != nil {
		return nil, fmt.Errorf("failed to write chat_id field: %w", err)
	}

	// Add caption field if content exists
	if post.Content != "" {
		if err := writer.WriteField("caption", post.Content); err != nil {
			return nil, fmt.Errorf("failed to write caption field: %w", err)
		}
	}

	// Add photo field
	part, err := writer.CreateFormFile("photo", "image.jpg")
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := part.Write(mediaData); err != nil {
		return nil, fmt.Errorf("failed to write media data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var telegramResp TelegramResponse
	if err := json.Unmarshal(body, &telegramResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !telegramResp.Ok {
		return nil, fmt.Errorf("telegram API error %d: %s", telegramResp.ErrorCode, telegramResp.Description)
	}

	var message TelegramMessage
	if err := json.Unmarshal(telegramResp.Result, &message); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &message, nil
}

// makeAPICall makes a generic API call to Telegram
func (t *Telegram) makeAPICall(ctx context.Context, url string, payload map[string]interface{}) (*TelegramMessage, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var telegramResp TelegramResponse
	if err := json.Unmarshal(body, &telegramResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !telegramResp.Ok {
		return nil, fmt.Errorf("telegram API error %d: %s", telegramResp.ErrorCode, telegramResp.Description)
	}

	var message TelegramMessage
	if err := json.Unmarshal(telegramResp.Result, &message); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &message, nil
}

// ListPosts retrieves recent messages from the Telegram chat
func (t *Telegram) ListPosts(ctx context.Context, limit int) ([]*Post, error) {
	// Set default limit if not specified
	if limit <= 0 {
		limit = 20
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates", t.Token)

	payload := map[string]interface{}{
		"limit": limit,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var telegramResp TelegramResponse
	if err := json.Unmarshal(body, &telegramResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !telegramResp.Ok {
		return nil, fmt.Errorf("telegram API error %d: %s", telegramResp.ErrorCode, telegramResp.Description)
	}

	var updates []TelegramGetUpdatesResponse
	if err := json.Unmarshal(telegramResp.Result, &updates); err != nil {
		return nil, fmt.Errorf("failed to unmarshal updates: %w", err)
	}

	// Convert Telegram messages to our Post type
	posts := make([]*Post, 0, len(updates))
	for _, update := range updates {
		if update.Message == nil {
			continue
		}

		message := update.Message

		// Skip messages not from our target chat
		chatIDStr := strconv.FormatInt(message.Chat.ID, 10)
		if chatIDStr != t.ChatID {
			continue
		}

		// Determine visibility based on chat type
		visibility := VisibilityLevelPublic
		if message.Chat.Type == "private" {
			visibility = VisibilityLevelPrivate
		}

		// Get content (text or caption)
		content := message.Text
		if content == "" {
			content = message.Caption
		}

		post := &Post{
			ID:             strconv.Itoa(message.MessageID),
			Content:        content,
			Visibility:     visibility,
			SourcePlatform: PlatformTelegram.String(),
			OriginalID:     strconv.Itoa(message.MessageID),
			CreatedAt:      time.Unix(message.Date, 0),
		}

		// Add media if present
		if len(message.Photo) > 0 {
			// Use the largest photo size
			var largestPhoto TelegramPhotoSize
			for _, photo := range message.Photo {
				if photo.Width > largestPhoto.Width {
					largestPhoto = photo
				}
			}

			// We can't easily get the actual photo data, so we'll create a placeholder
			// In a real implementation, you'd use getFile API to get the file path
			post.Media = []Media{*NewMediaFromURL("")}
		}

		posts = append(posts, post)
	}

	return posts, nil
}
