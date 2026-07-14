package social

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"

	"butterfly.orx.me/core/log"

	"go.orx.me/apps/hyper-sync/internal/media"
)

// SyncCursorDao persists polling offsets for pull-based content sources.
type SyncCursorDao interface {
	GetOffset(ctx context.Context, platform string) (int64, error)
	SaveOffset(ctx context.Context, platform string, offset int64) error
}

// mediaGroupFlushDelay is how long TelegramClient waits after the last part
// of a media group before treating the group as complete. Telegram delivers
// the messages of an album back-to-back, so a short quiet period is enough
// to know no more parts are coming.
const mediaGroupFlushDelay = 700 * time.Millisecond

// pendingMediaGroup tracks an in-progress media group being assembled from
// multiple Telegram updates sharing the same media_group_id.
type pendingMediaGroup struct {
	post  *Post
	timer *time.Timer
}

// TelegramClient implements SocialClient for Telegram channel ingestion.
//
// It runs a go-telegram/bot long-polling loop in the background for the
// lifetime of the client; ListPosts just drains the posts that loop has
// buffered so far. The polling offset is persisted via cursor after every
// processed update so a restart resumes where it left off.
type TelegramClient struct {
	bot           *tgbot.Bot
	name          string
	cursor        SyncCursorDao
	objectStorage media.ObjectStorage
	cdnDomain     string

	cancel context.CancelFunc

	mu            sync.Mutex
	buffer        []*Post
	pendingGroups map[string]*pendingMediaGroup
}

func NewTelegramClient(botToken, channelID, name, apiBase string, cursor SyncCursorDao, objectStorage media.ObjectStorage, cdnDomain string) (*TelegramClient, error) {
	logger := slog.Default()

	t := &TelegramClient{
		name:          name,
		cursor:        cursor,
		objectStorage: objectStorage,
		cdnDomain:     cdnDomain,
		pendingGroups: make(map[string]*pendingMediaGroup),
	}

	var offset int64
	if cursor != nil {
		var err error
		offset, err = cursor.GetOffset(context.Background(), name)
		if err != nil {
			return nil, fmt.Errorf("telegram: get offset: %w", err)
		}
		logger.Info("loaded polling offset",
			"client", name,
			"offset", offset)
	}

	opts := []tgbot.Option{
		tgbot.WithSkipGetMe(),
		tgbot.WithNotAsyncHandlers(),
		tgbot.WithDefaultHandler(t.handleUpdate),
		tgbot.WithAllowedUpdates(tgbot.AllowedUpdates{"channel_post"}),
	}
	if offset > 0 {
		// The bot library's getUpdates loop always requests lastUpdateID+1,
		// so seed it one below our stored "next offset to fetch" value.
		opts = append(opts, tgbot.WithInitialOffset(offset-1))
	}
	if apiBase != "" {
		opts = append(opts, tgbot.WithServerURL(apiBase))
	}

	b, err := tgbot.New(botToken, opts...)
	if err != nil {
		return nil, fmt.Errorf("telegram: create bot: %w", err)
	}
	t.bot = b

	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel
	go b.Start(ctx)

	logger.Info("telegram client started",
		"client", name,
		"api_base", apiBase,
		"has_object_storage", objectStorage != nil)

	return t, nil
}

// Close stops the background polling loop.
func (t *TelegramClient) Close() {
	if t.cancel != nil {
		t.cancel()
	}
}

func (t *TelegramClient) Name() string { return t.name }

func (t *TelegramClient) Post(_ context.Context, _ *Post) (interface{}, error) {
	return nil, fmt.Errorf("telegram: posting not implemented")
}

// Compile-time check that TelegramClient satisfies SocialClient.
var _ SocialClient = (*TelegramClient)(nil)

// ListPosts drains the posts buffered by the background polling loop since
// the last call. limit <= 0 means "return everything buffered".
func (t *TelegramClient) ListPosts(ctx context.Context, limit int) ([]*Post, error) {
	logger := log.FromContext(ctx)

	t.mu.Lock()
	defer t.mu.Unlock()

	if limit <= 0 || limit > len(t.buffer) {
		limit = len(t.buffer)
	}
	posts := t.buffer[:limit]
	t.buffer = t.buffer[limit:]

	logger.Debug("drained buffered posts",
		"client", t.name,
		"returned", len(posts),
		"remaining", len(t.buffer))

	return posts, nil
}

func (t *TelegramClient) handleUpdate(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	logger := log.FromContext(ctx)

	logger.Debug("received update",
		"client", t.name,
		"update_id", update.ID)

	if msg := update.ChannelPost; msg != nil {
		logger.Info("received channel post",
			"client", t.name,
			"message_id", msg.ID,
			"has_text", msg.Text != "",
			"has_caption", msg.Caption != "",
			"has_photo", len(msg.Photo) > 0,
			"has_video", msg.Video != nil,
			"media_group_id", msg.MediaGroupID)
		t.ingest(ctx, msg)
	}

	if t.cursor != nil {
		if err := t.cursor.SaveOffset(ctx, t.name, update.ID+1); err != nil {
			logger.Error("failed to save offset",
				"client", t.name,
				"offset", update.ID+1,
				"error", err)
		}
	}
}

// ingest merges media-group parts and appends completed posts to buffer.
func (t *TelegramClient) ingest(ctx context.Context, msg *models.Message) {
	logger := log.FromContext(ctx)

	t.mu.Lock()
	defer t.mu.Unlock()

	if msg.MediaGroupID != "" {
		if pg, ok := t.pendingGroups[msg.MediaGroupID]; ok {
			logger.Debug("merging into existing media group",
				"client", t.name,
				"message_id", msg.ID,
				"media_group_id", msg.MediaGroupID)
			t.mergeIntoPendingLocked(ctx, pg, msg)
			return
		}

		post := t.convert(ctx, msg)
		if post == nil {
			logger.Debug("skipped message with no recognizable content",
				"client", t.name,
				"message_id", msg.ID)
			return
		}
		logger.Debug("started new media group",
			"client", t.name,
			"message_id", msg.ID,
			"media_group_id", msg.MediaGroupID)
		pg := &pendingMediaGroup{post: post}
		t.pendingGroups[msg.MediaGroupID] = pg
		t.scheduleFlushLocked(msg.MediaGroupID, pg)
		return
	}

	post := t.convert(ctx, msg)
	if post == nil {
		logger.Debug("skipped message with no recognizable content",
			"client", t.name,
			"message_id", msg.ID)
		return
	}
	t.buffer = append(t.buffer, post)
	logger.Info("buffered post",
		"client", t.name,
		"post_id", post.ID,
		"content_len", len(post.Content),
		"media_count", len(post.Media))
}

func (t *TelegramClient) mergeIntoPendingLocked(ctx context.Context, pg *pendingMediaGroup, msg *models.Message) {
	for _, url := range t.mediaURLs(ctx, msg) {
		pg.post.Media = append(pg.post.Media, *NewMediaFromURL(url))
	}
	if pg.post.Content == "" && msg.Caption != "" {
		pg.post.Content = stripEntities(msg.Caption, msg.CaptionEntities)
	}
	t.scheduleFlushLocked(msg.MediaGroupID, pg)
}

func (t *TelegramClient) scheduleFlushLocked(groupID string, pg *pendingMediaGroup) {
	if pg.timer != nil {
		pg.timer.Stop()
	}
	pg.timer = time.AfterFunc(mediaGroupFlushDelay, func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		t.flushGroupLocked(groupID)
	})
}

func (t *TelegramClient) flushGroupLocked(groupID string) {
	pg, ok := t.pendingGroups[groupID]
	if !ok {
		return
	}
	t.buffer = append(t.buffer, pg.post)
	delete(t.pendingGroups, groupID)

	slog.Default().Info("flushed media group",
		"client", t.name,
		"media_group_id", groupID,
		"post_id", pg.post.ID,
		"content_len", len(pg.post.Content),
		"media_count", len(pg.post.Media))
}

// convert builds a Post from a Telegram message. Returns nil if the message
// carries no content HyperSync understands (e.g. sticker, poll, document).
func (t *TelegramClient) convert(ctx context.Context, msg *models.Message) *Post {
	logger := log.FromContext(ctx)

	content := stripEntities(msg.Text, msg.Entities)
	if content == "" {
		content = stripEntities(msg.Caption, msg.CaptionEntities)
	}

	mediaURLs := t.mediaURLs(ctx, msg)
	if content == "" && len(mediaURLs) == 0 {
		return nil
	}

	var media []Media
	for _, url := range mediaURLs {
		media = append(media, *NewMediaFromURL(url))
	}

	id := strconv.Itoa(msg.ID)
	logger.Debug("converted message to post",
		"client", t.name,
		"message_id", msg.ID,
		"content_len", len(content),
		"media_count", len(media))

	return &Post{
		ID:             id,
		Content:        content,
		Visibility:     VisibilityLevelPublic,
		Media:          media,
		SourcePlatform: t.name,
		OriginalID:     id,
		CreatedAt:      time.Unix(int64(msg.Date), 0).UTC(),
	}
}

// mediaURLs downloads a message's photo (largest size only) and/or video, if
// present, and uploads each to object storage, returning their permanent
// CDN URLs. Telegram's own temporary file-serving URLs are never returned.
func (t *TelegramClient) mediaURLs(ctx context.Context, msg *models.Message) []string {
	logger := log.FromContext(ctx)
	var urls []string

	if len(msg.Photo) > 0 {
		largest := msg.Photo[len(msg.Photo)-1]
		logger.Debug("downloading photo",
			"client", t.name,
			"message_id", msg.ID,
			"file_id", largest.FileID)
		if url, err := t.downloadAndStoreFile(ctx, largest.FileID); err == nil {
			logger.Info("photo uploaded to CDN",
				"client", t.name,
				"message_id", msg.ID,
				"url", url)
			urls = append(urls, url)
		} else {
			logger.Error("failed to get photo file",
				"client", t.name,
				"message_id", msg.ID,
				"error", err)
		}
	}

	if msg.Video != nil {
		logger.Debug("downloading video",
			"client", t.name,
			"message_id", msg.ID,
			"file_id", msg.Video.FileID)
		if url, err := t.downloadAndStoreFile(ctx, msg.Video.FileID); err == nil {
			logger.Info("video uploaded to CDN",
				"client", t.name,
				"message_id", msg.ID,
				"url", url)
			urls = append(urls, url)
		} else {
			logger.Error("failed to get video file",
				"client", t.name,
				"message_id", msg.ID,
				"error", err)
		}
	}

	return urls
}

// downloadAndStoreFile fetches a Telegram file by ID via the Bot API,
// downloads its bytes over Telegram's temporary file-serving URL, uploads
// them to object storage, and returns the permanent CDN URL.
func (t *TelegramClient) downloadAndStoreFile(ctx context.Context, fileID string) (string, error) {
	logger := log.FromContext(ctx)

	if t.objectStorage == nil || t.cdnDomain == "" {
		return "", fmt.Errorf("telegram: no object storage configured")
	}

	file, err := t.bot.GetFile(ctx, &tgbot.GetFileParams{FileID: fileID})
	if err != nil {
		return "", fmt.Errorf("telegram: get file: %w", err)
	}
	tempURL := t.bot.FileDownloadLink(file)

	logger.Debug("downloading file from Telegram",
		"client", t.name,
		"file_id", fileID,
		"file_path", file.FilePath)

	resp, err := http.Get(tempURL)
	if err != nil {
		return "", fmt.Errorf("telegram: download file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("telegram: download file: status code %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("telegram: read file: %w", err)
	}

	contentType := http.DetectContentType(data)
	key := fmt.Sprintf("telegram/%s/%s-%s", time.Now().Format("2006/01/02"), uuid.New().String(), filepath.Base(file.FilePath))

	logger.Debug("uploading file to object storage",
		"client", t.name,
		"file_id", fileID,
		"content_type", contentType,
		"size", len(data),
		"key", key)

	if err := t.objectStorage.Upload(ctx, key, contentType, bytes.NewReader(data)); err != nil {
		return "", fmt.Errorf("telegram: upload file: %w", err)
	}

	cdnURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(t.cdnDomain, "/"), key)
	logger.Debug("file stored successfully",
		"client", t.name,
		"file_id", fileID,
		"cdn_url", cdnURL)

	return cdnURL, nil
}

// stripEntities processes Telegram entities on a text string.
// Most entities (bold, italic, mention, etc.) are just metadata — the text
// field is already plain. text_link entities carry a hidden URL that would
// be lost, so we insert it after the link text.
//
// Telegram entity offset/length are UTF-16 code units, so the text is
// worked on as UTF-16 units rather than runes.
func stripEntities(text string, entities []models.MessageEntity) string {
	if len(entities) == 0 {
		return text
	}

	units := utf16.Encode([]rune(text))

	type insertion struct {
		pos  int
		text string
	}
	var insertions []insertion
	for _, e := range entities {
		if e.Type == models.MessageEntityTypeTextLink && e.URL != "" {
			insertions = append(insertions, insertion{
				pos:  e.Offset + e.Length,
				text: " (" + e.URL + ")",
			})
		}
	}

	// Sort by position descending so we can insert without shifting earlier positions.
	for i := 0; i < len(insertions); i++ {
		for j := i + 1; j < len(insertions); j++ {
			if insertions[j].pos > insertions[i].pos {
				insertions[i], insertions[j] = insertions[j], insertions[i]
			}
		}
	}

	for _, ins := range insertions {
		pos := ins.pos
		if pos > len(units) {
			pos = len(units)
		}
		insUnits := utf16.Encode([]rune(ins.text))
		expanded := make([]uint16, 0, len(units)+len(insUnits))
		expanded = append(expanded, units[:pos]...)
		expanded = append(expanded, insUnits...)
		expanded = append(expanded, units[pos:]...)
		units = expanded
	}

	return string(utf16.Decode(units))
}
