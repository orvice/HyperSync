package social

import (
	"context"
	"fmt"
	"strconv"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Telegram struct {
	bot    *tgbotapi.BotAPI
	ChatID string
	name   string
}

func NewTelegram(token, chatID, name string) (*Telegram, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create Telegram bot: %w", err)
	}

	return &Telegram{
		bot:    bot,
		ChatID: chatID,
		name:   name,
	}, nil
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

	// Parse chat ID to int64
	chatID, err := strconv.ParseInt(t.ChatID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid chat ID '%s': %w", t.ChatID, err)
	}

	// If there are media attachments, send as photo/document with caption
	if len(post.Media) > 0 {
		return t.sendMediaMessage(ctx, chatID, post)
	}

	// Send as text message
	return t.sendTextMessage(ctx, chatID, post.Content)
}

// sendTextMessage sends a text message to Telegram
func (t *Telegram) sendTextMessage(_ context.Context, chatID int64, text string) (*tgbotapi.Message, error) {
	msg := tgbotapi.NewMessage(chatID, text)

	sentMsg, err := t.bot.Send(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to send text message: %w", err)
	}

	return &sentMsg, nil
}

// sendMediaMessage sends a message with media to Telegram
func (t *Telegram) sendMediaMessage(ctx context.Context, chatID int64, post *Post) (*tgbotapi.Message, error) {
	// For now, we'll send the first media attachment as a photo with caption
	// TODO: Support multiple media attachments
	if len(post.Media) == 0 {
		return t.sendTextMessage(ctx, chatID, post.Content)
	}

	media := post.Media[0]
	mediaData, err := media.GetData()
	if err != nil {
		return nil, fmt.Errorf("failed to get media data: %w", err)
	}

	// Create photo message with media data
	photoFileBytes := tgbotapi.FileBytes{
		Name:  "image.jpg",
		Bytes: mediaData,
	}

	msg := tgbotapi.NewPhoto(chatID, photoFileBytes)

	// Add caption if content exists
	if post.Content != "" {
		msg.Caption = post.Content
	}

	sentMsg, err := t.bot.Send(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to send photo message: %w", err)
	}

	return &sentMsg, nil
}

// ListPosts retrieves recent messages from the Telegram chat
func (t *Telegram) ListPosts(ctx context.Context, limit int) ([]*Post, error) {
	// Set default limit if not specified
	if limit <= 0 {
		limit = 20
	}

	// Parse chat ID to int64
	chatID, err := strconv.ParseInt(t.ChatID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid chat ID '%s': %w", t.ChatID, err)
	}

	// Create update config
	u := tgbotapi.NewUpdate(0)
	u.Limit = limit
	u.Timeout = 60

	// Get updates
	updates, err := t.bot.GetUpdates(u)
	if err != nil {
		return nil, fmt.Errorf("failed to get updates: %w", err)
	}

	// Convert Telegram messages to our Post type
	posts := make([]*Post, 0, len(updates))
	for _, update := range updates {
		if update.Message == nil {
			continue
		}

		message := update.Message

		// Skip messages not from our target chat
		if message.Chat.ID != chatID {
			continue
		}

		// Determine visibility based on chat type
		visibility := VisibilityLevelPublic
		if message.Chat.IsPrivate() {
			visibility = VisibilityLevelPrivate
		}

		// Get content (text or caption)
		content := message.Text
		if content == "" && message.Caption != "" {
			content = message.Caption
		}

		post := &Post{
			ID:             strconv.Itoa(message.MessageID),
			Content:        content,
			Visibility:     visibility,
			SourcePlatform: PlatformTelegram.String(),
			OriginalID:     strconv.Itoa(message.MessageID),
			CreatedAt:      time.Unix(int64(message.Date), 0),
		}

		// Add media if present
		if len(message.Photo) > 0 {
			// Use the largest photo size
			var largestPhoto tgbotapi.PhotoSize
			for _, photo := range message.Photo {
				if photo.Width > largestPhoto.Width {
					largestPhoto = photo
				}
			}

			// Create media with file ID (we can't easily get the actual data without additional API calls)
			post.Media = []Media{*NewMediaFromURL("")} // Placeholder for now
		}

		posts = append(posts, post)
	}

	return posts, nil
}
