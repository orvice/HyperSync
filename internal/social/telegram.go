package social

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
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

	mu             sync.Mutex
	buffer         []*Post
	pendingGroupID string
	pendingGroup   *Post
	flushTimer     *time.Timer
}

func NewTelegramClient(botToken, channelID, name, apiBase string, cursor SyncCursorDao, objectStorage media.ObjectStorage, cdnDomain string) (*TelegramClient, error) {
	t := &TelegramClient{
		name:          name,
		cursor:        cursor,
		objectStorage: objectStorage,
		cdnDomain:     cdnDomain,
	}

	var offset int64
	if cursor != nil {
		var err error
		offset, err = cursor.GetOffset(context.Background(), name)
		if err != nil {
			return nil, fmt.Errorf("telegram: get offset: %w", err)
		}
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
func (t *TelegramClient) ListPosts(_ context.Context, limit int) ([]*Post, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if limit <= 0 || limit > len(t.buffer) {
		limit = len(t.buffer)
	}
	posts := t.buffer[:limit]
	t.buffer = t.buffer[limit:]
	return posts, nil
}

func (t *TelegramClient) handleUpdate(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	if msg := update.ChannelPost; msg != nil {
		t.ingest(ctx, msg)
	}

	if t.cursor != nil {
		if err := t.cursor.SaveOffset(ctx, t.name, update.ID+1); err != nil {
			log.Printf("telegram: save offset for %s: %v", t.name, err)
		}
	}
}

// ingest merges media-group parts and appends completed posts to buffer.
func (t *TelegramClient) ingest(ctx context.Context, msg *models.Message) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if msg.MediaGroupID != "" && t.pendingGroup != nil && t.pendingGroupID == msg.MediaGroupID {
		t.mergeIntoPendingGroupLocked(ctx, msg)
		return
	}

	t.flushPendingGroupLocked()

	post := t.convert(ctx, msg)
	if post == nil {
		return
	}

	if msg.MediaGroupID == "" {
		t.buffer = append(t.buffer, post)
		return
	}

	t.pendingGroupID = msg.MediaGroupID
	t.pendingGroup = post
	t.scheduleFlushLocked()
}

func (t *TelegramClient) mergeIntoPendingGroupLocked(ctx context.Context, msg *models.Message) {
	for _, url := range t.mediaURLs(ctx, msg) {
		t.pendingGroup.Media = append(t.pendingGroup.Media, *NewMediaFromURL(url))
	}
	if t.pendingGroup.Content == "" && msg.Caption != "" {
		t.pendingGroup.Content = stripEntities(msg.Caption, msg.CaptionEntities)
	}
	t.scheduleFlushLocked()
}

func (t *TelegramClient) scheduleFlushLocked() {
	if t.flushTimer != nil {
		t.flushTimer.Stop()
	}
	t.flushTimer = time.AfterFunc(mediaGroupFlushDelay, func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		t.flushPendingGroupLocked()
	})
}

func (t *TelegramClient) flushPendingGroupLocked() {
	if t.pendingGroup == nil {
		return
	}
	t.buffer = append(t.buffer, t.pendingGroup)
	t.pendingGroup = nil
	t.pendingGroupID = ""
}

// convert builds a Post from a Telegram message. Returns nil if the message
// carries no content HyperSync understands (e.g. sticker, poll, document).
func (t *TelegramClient) convert(ctx context.Context, msg *models.Message) *Post {
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
	var urls []string

	if len(msg.Photo) > 0 {
		largest := msg.Photo[len(msg.Photo)-1]
		if url, err := t.downloadAndStoreFile(ctx, largest.FileID); err == nil {
			urls = append(urls, url)
		} else {
			log.Printf("telegram: get photo file: %v", err)
		}
	}

	if msg.Video != nil {
		if url, err := t.downloadAndStoreFile(ctx, msg.Video.FileID); err == nil {
			urls = append(urls, url)
		} else {
			log.Printf("telegram: get video file: %v", err)
		}
	}

	return urls
}

// downloadAndStoreFile fetches a Telegram file by ID via the Bot API,
// downloads its bytes over Telegram's temporary file-serving URL, uploads
// them to object storage, and returns the permanent CDN URL.
func (t *TelegramClient) downloadAndStoreFile(ctx context.Context, fileID string) (string, error) {
	if t.objectStorage == nil || t.cdnDomain == "" {
		return "", fmt.Errorf("telegram: no object storage configured")
	}

	file, err := t.bot.GetFile(ctx, &tgbot.GetFileParams{FileID: fileID})
	if err != nil {
		return "", fmt.Errorf("telegram: get file: %w", err)
	}
	tempURL := t.bot.FileDownloadLink(file)

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
	if err := t.objectStorage.Upload(ctx, key, contentType, bytes.NewReader(data)); err != nil {
		return "", fmt.Errorf("telegram: upload file: %w", err)
	}

	return fmt.Sprintf("%s/%s", strings.TrimSuffix(t.cdnDomain, "/"), key), nil
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
