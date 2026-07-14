package social

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.orx.me/apps/hyper-sync/internal/media"
)

// fakeTelegramServer emulates the subset of the Telegram Bot API that
// go-telegram/bot's long-polling loop and file lookups exercise. Batches of
// updates are queued and handed out one per getUpdates request, in order;
// once the queue is empty it keeps returning an empty result (like Telegram
// does when there's nothing new).
type fakeTelegramServer struct {
	*httptest.Server

	mu             sync.Mutex
	batches        [][]map[string]any
	files          map[string]string // file_id -> file_path
	requestOffsets []string
}

func newFakeTelegramServer(t *testing.T) *fakeTelegramServer {
	t.Helper()
	f := &fakeTelegramServer{files: make(map[string]string)}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeTelegramServer) handle(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/getUpdates"):
		f.mu.Lock()
		f.requestOffsets = append(f.requestOffsets, r.FormValue("offset"))
		var batch []map[string]any
		if len(f.batches) > 0 {
			batch = f.batches[0]
			f.batches = f.batches[1:]
		}
		f.mu.Unlock()

		// Throttle so an empty-queue loop doesn't spin at full CPU.
		if batch == nil {
			time.Sleep(15 * time.Millisecond)
			batch = []map[string]any{}
		}
		writeJSON(w, map[string]any{"ok": true, "result": batch})

	case strings.HasSuffix(r.URL.Path, "/getFile"):
		fileID := r.FormValue("file_id")
		f.mu.Lock()
		path := f.files[fileID]
		f.mu.Unlock()
		writeJSON(w, map[string]any{
			"ok":     true,
			"result": map[string]any{"file_id": fileID, "file_path": path},
		})

	case strings.Contains(r.URL.Path, "/file/bot"):
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("fake-file-bytes"))

	default:
		http.NotFound(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeTelegramServer) pushBatch(batch []map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.batches = append(f.batches, batch)
}

func (f *fakeTelegramServer) setFile(fileID, filePath string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[fileID] = filePath
}

func (f *fakeTelegramServer) offsets() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requestOffsets...)
}

// waitForPosts polls ListPosts until at least want posts have been
// collected or timeout elapses.
func waitForPosts(t *testing.T, client *TelegramClient, want int, timeout time.Duration) []*Post {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var collected []*Post
	for time.Now().Before(deadline) {
		posts, err := client.ListPosts(context.Background(), 0)
		require.NoError(t, err)
		collected = append(collected, posts...)
		if len(collected) >= want {
			return collected
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d posts, got %d: %+v", want, len(collected), collected)
	return nil
}

// assertNoMorePosts checks ListPosts stays empty for a short grace period.
func assertNoMorePosts(t *testing.T, client *TelegramClient, wait time.Duration) {
	t.Helper()
	time.Sleep(wait)
	posts, err := client.ListPosts(context.Background(), 0)
	require.NoError(t, err)
	assert.Empty(t, posts)
}

func TestTelegram_ListPosts_SingleTextMessage(t *testing.T) {
	msgDate := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	server := newFakeTelegramServer(t)
	server.pushBatch([]map[string]any{
		{
			"update_id": 100,
			"channel_post": map[string]any{
				"message_id": 42,
				"date":       msgDate.Unix(),
				"text":       "Hello from Telegram",
				"chat":       map[string]any{"id": -1001234567890, "type": "channel"},
			},
		},
	})

	client, err := NewTelegramClient("test-token", "-1001234567890", "my-telegram", server.URL, nil, nil, "")
	require.NoError(t, err)
	defer client.Close()

	posts := waitForPosts(t, client, 1, 3*time.Second)
	require.Len(t, posts, 1)

	post := posts[0]
	assert.Equal(t, "Hello from Telegram", post.Content)
	assert.Equal(t, "42", post.OriginalID)
	assert.Equal(t, "42", post.ID)
	assert.Equal(t, VisibilityLevelPublic, post.Visibility)
	assert.Equal(t, "my-telegram", post.SourcePlatform)
	assert.Equal(t, msgDate, post.CreatedAt)
	assert.Empty(t, post.Media)
}

func TestTelegram_ListPosts_PhotoWithCaption(t *testing.T) {
	msgDate := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	server := newFakeTelegramServer(t)
	server.setFile("large_id", "photos/file_123.jpg")
	server.pushBatch([]map[string]any{
		{
			"update_id": 200,
			"channel_post": map[string]any{
				"message_id": 55,
				"date":       msgDate.Unix(),
				"caption":    "A beautiful sunset",
				"chat":       map[string]any{"id": -1001234567890, "type": "channel"},
				"photo": []map[string]any{
					{"file_id": "small_id", "file_unique_id": "s1", "width": 90, "height": 90},
					{"file_id": "medium_id", "file_unique_id": "s2", "width": 320, "height": 320},
					{"file_id": "large_id", "file_unique_id": "s3", "width": 800, "height": 800},
				},
			},
		},
	})

	storage := media.NewMemoryObjectStorage()
	client, err := NewTelegramClient("test-token", "-1001234567890", "my-telegram", server.URL, nil, storage, "https://cdn.example.com")
	require.NoError(t, err)
	defer client.Close()

	posts := waitForPosts(t, client, 1, 3*time.Second)
	require.Len(t, posts, 1)

	post := posts[0]
	assert.Equal(t, "A beautiful sunset", post.Content)
	assert.Equal(t, "55", post.OriginalID)
	assert.Equal(t, VisibilityLevelPublic, post.Visibility)
	require.Len(t, post.Media, 1)

	url := post.Media[0].GetURL()
	assert.True(t, strings.HasPrefix(url, "https://cdn.example.com/"))
	assert.Contains(t, url, "file_123.jpg")
	assert.NotContains(t, url, "api.telegram.org")
	assert.NotContains(t, url, server.URL)

	key := strings.TrimPrefix(url, "https://cdn.example.com/")
	assert.True(t, storage.Has(key), "uploaded file should be present in object storage")
}

func TestTelegram_ListPosts_SkippedMessageTypes(t *testing.T) {
	server := newFakeTelegramServer(t)
	server.pushBatch([]map[string]any{
		{
			"update_id": 300,
			"channel_post": map[string]any{
				"message_id": 60,
				"date":       1000,
				"sticker":    map[string]any{"file_id": "sticker1", "file_unique_id": "u1", "width": 1, "height": 1, "is_animated": false, "is_video": false},
				"chat":       map[string]any{"id": -100, "type": "channel"},
			},
		},
		{
			"update_id": 301,
			"channel_post": map[string]any{
				"message_id": 61,
				"date":       1001,
				"poll":       map[string]any{"id": "p1", "question": "yes?", "options": []map[string]any{}, "total_voter_count": 0, "is_closed": false, "is_anonymous": true, "type": "regular", "allows_multiple_answers": false},
				"chat":       map[string]any{"id": -100, "type": "channel"},
			},
		},
		{
			"update_id": 302,
			"channel_post": map[string]any{
				"message_id": 62,
				"date":       1002,
				"document":   map[string]any{"file_id": "doc1", "file_unique_id": "u2"},
				"chat":       map[string]any{"id": -100, "type": "channel"},
			},
		},
		{
			"update_id": 303,
			"channel_post": map[string]any{
				"message_id": 63,
				"date":       1003,
				"audio":      map[string]any{"file_id": "audio1", "file_unique_id": "u3", "duration": 10},
				"chat":       map[string]any{"id": -100, "type": "channel"},
			},
		},
	})

	client, err := NewTelegramClient("test-token", "-100", "tg", server.URL, nil, nil, "")
	require.NoError(t, err)
	defer client.Close()

	assertNoMorePosts(t, client, 500*time.Millisecond)
}

// memoryCursor is an in-memory SyncCursorDao for testing.
type memoryCursor struct {
	mu      sync.Mutex
	offsets map[string]int64
}

func newMemoryCursor() *memoryCursor {
	return &memoryCursor{offsets: make(map[string]int64)}
}

func (m *memoryCursor) GetOffset(_ context.Context, platform string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.offsets[platform], nil
}

func (m *memoryCursor) SaveOffset(_ context.Context, platform string, offset int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.offsets[platform] = offset
	return nil
}

func (m *memoryCursor) get(platform string) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.offsets[platform]
}

func TestTelegram_ListPosts_OffsetAdvancement(t *testing.T) {
	server := newFakeTelegramServer(t)
	server.pushBatch([]map[string]any{
		{
			"update_id": 100,
			"channel_post": map[string]any{
				"message_id": 1, "date": 1000, "text": "first",
				"chat": map[string]any{"id": -100, "type": "channel"},
			},
		},
		{
			"update_id": 105,
			"channel_post": map[string]any{
				"message_id": 2, "date": 1001, "text": "second",
				"chat": map[string]any{"id": -100, "type": "channel"},
			},
		},
	})

	cursor := newMemoryCursor()
	client, err := NewTelegramClient("test-token", "-100", "tg", server.URL, cursor, nil, "")
	require.NoError(t, err)
	defer client.Close()

	posts := waitForPosts(t, client, 2, 3*time.Second)
	assert.Len(t, posts, 2)

	require.Eventually(t, func() bool {
		return cursor.get("tg") == 106
	}, 2*time.Second, 20*time.Millisecond, "offset should advance to max(update_id)+1")

	offsets := server.offsets()
	require.NotEmpty(t, offsets)
	assert.Equal(t, "1", offsets[0], "first request should start from offset 1 with no persisted cursor")
	assert.Contains(t, offsets, "106", "a later request should carry offset=106")
}

func TestTelegram_ListPosts_EmptyBatch(t *testing.T) {
	server := newFakeTelegramServer(t)

	cursor := newMemoryCursor()
	cursor.offsets["tg"] = 50 // pre-existing offset

	client, err := NewTelegramClient("test-token", "-100", "tg", server.URL, cursor, nil, "")
	require.NoError(t, err)
	defer client.Close()

	require.Eventually(t, func() bool {
		offsets := server.offsets()
		return len(offsets) > 0 && offsets[0] == "50"
	}, 2*time.Second, 20*time.Millisecond, "first request should carry the persisted offset")

	assertNoMorePosts(t, client, 200*time.Millisecond)
	assert.Equal(t, int64(50), cursor.get("tg"), "offset should not change on empty batches")
}

func TestTelegram_ListPosts_EntityStripping(t *testing.T) {
	// Telegram's text field is already plain text. Entities like bold/italic
	// are just metadata annotations — the text itself doesn't contain markup.
	// text_link is special: it associates a URL with a text range. We want
	// to append the URL after the link text so it's not lost in plain text.
	server := newFakeTelegramServer(t)
	server.pushBatch([]map[string]any{
		{
			"update_id": 400,
			"channel_post": map[string]any{
				"message_id": 70,
				"date":       1000,
				"text":       "Check this link and bold text here",
				"chat":       map[string]any{"id": -100, "type": "channel"},
				"entities": []map[string]any{
					{"type": "text_link", "offset": 11, "length": 4, "url": "https://example.com"},
					{"type": "bold", "offset": 20, "length": 4},
					{"type": "mention", "offset": 30, "length": 4},
				},
			},
		},
	})

	client, err := NewTelegramClient("test-token", "-100", "tg", server.URL, nil, nil, "")
	require.NoError(t, err)
	defer client.Close()

	posts := waitForPosts(t, client, 1, 3*time.Second)
	require.Len(t, posts, 1)

	assert.Equal(t, "Check this link (https://example.com) and bold text here", posts[0].Content)
}

func TestTelegram_ListPosts_MediaGroupMerge(t *testing.T) {
	msgDate := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	server := newFakeTelegramServer(t)
	server.setFile("p1", "photos/p1.jpg")
	server.setFile("p2", "photos/p2.jpg")
	server.pushBatch([]map[string]any{
		{
			"update_id": 500,
			"channel_post": map[string]any{
				"message_id":     70,
				"date":           msgDate.Unix(),
				"caption":        "Album title",
				"media_group_id": "album1",
				"chat":           map[string]any{"id": -100, "type": "channel"},
				"photo": []map[string]any{
					{"file_id": "p1", "file_unique_id": "u1", "width": 800, "height": 600},
				},
			},
		},
		{
			"update_id": 501,
			"channel_post": map[string]any{
				"message_id":     71,
				"date":           msgDate.Unix(),
				"media_group_id": "album1",
				"chat":           map[string]any{"id": -100, "type": "channel"},
				"photo": []map[string]any{
					{"file_id": "p2", "file_unique_id": "u2", "width": 800, "height": 600},
				},
			},
		},
	})

	storage := media.NewMemoryObjectStorage()
	client, err := NewTelegramClient("test-token", "-100", "tg", server.URL, nil, storage, "https://cdn.example.com")
	require.NoError(t, err)
	defer client.Close()

	posts := waitForPosts(t, client, 1, 3*time.Second)
	require.Len(t, posts, 1, "media group should merge into one post")

	post := posts[0]
	assert.Equal(t, "Album title", post.Content)
	require.Len(t, post.Media, 2)
	assert.Contains(t, post.Media[0].GetURL(), "p1")
	assert.Contains(t, post.Media[1].GetURL(), "p2")
	assert.NotContains(t, post.Media[0].GetURL(), "api.telegram.org")
	assert.True(t, strings.HasPrefix(post.Media[0].GetURL(), "https://cdn.example.com/"))
}

func TestTelegram_ListPosts_Video(t *testing.T) {
	msgDate := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	server := newFakeTelegramServer(t)
	server.setFile("video_id", "videos/clip.mp4")
	server.pushBatch([]map[string]any{
		{
			"update_id": 600,
			"channel_post": map[string]any{
				"message_id": 80,
				"date":       msgDate.Unix(),
				"caption":    "Watch this",
				"chat":       map[string]any{"id": -100, "type": "channel"},
				"video":      map[string]any{"file_id": "video_id", "file_unique_id": "v1", "width": 1280, "height": 720, "duration": 12},
			},
		},
	})

	storage := media.NewMemoryObjectStorage()
	client, err := NewTelegramClient("test-token", "-100", "tg", server.URL, nil, storage, "https://cdn.example.com")
	require.NoError(t, err)
	defer client.Close()

	posts := waitForPosts(t, client, 1, 3*time.Second)
	require.Len(t, posts, 1)

	post := posts[0]
	assert.Equal(t, "Watch this", post.Content)
	require.Len(t, post.Media, 1)

	url := post.Media[0].GetURL()
	assert.True(t, strings.HasPrefix(url, "https://cdn.example.com/"))
	assert.Contains(t, url, "clip.mp4")
	assert.NotContains(t, url, "api.telegram.org")
}

func TestTelegram_ListPosts_PhotoWithoutCaption(t *testing.T) {
	msgDate := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	server := newFakeTelegramServer(t)
	server.setFile("photo_id", "photos/no_caption.jpg")
	server.pushBatch([]map[string]any{
		{
			"update_id": 700,
			"channel_post": map[string]any{
				"message_id": 90,
				"date":       msgDate.Unix(),
				"chat":       map[string]any{"id": -100, "type": "channel"},
				"photo": []map[string]any{
					{"file_id": "photo_id", "file_unique_id": "p1", "width": 800, "height": 600},
				},
			},
		},
	})

	storage := media.NewMemoryObjectStorage()
	client, err := NewTelegramClient("test-token", "-100", "tg", server.URL, nil, storage, "https://cdn.example.com")
	require.NoError(t, err)
	defer client.Close()

	posts := waitForPosts(t, client, 1, 3*time.Second)
	require.Len(t, posts, 1)

	post := posts[0]
	assert.Equal(t, "", post.Content)
	require.Len(t, post.Media, 1)
	assert.Contains(t, post.Media[0].GetURL(), "no_caption.jpg")
}

func TestTelegram_ListPosts_DownloadFailureSkipsMessage(t *testing.T) {
	msgDate := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	server := newFakeTelegramServer(t)
	// photo_id is never registered via setFile, so getFile returns an empty
	// file_path and the download step fails.
	server.pushBatch([]map[string]any{
		{
			"update_id": 800,
			"channel_post": map[string]any{
				"message_id": 100,
				"date":       msgDate.Unix(),
				"caption":    "Broken photo",
				"chat":       map[string]any{"id": -100, "type": "channel"},
				"photo": []map[string]any{
					{"file_id": "missing_id", "file_unique_id": "m1", "width": 800, "height": 600},
				},
			},
		},
		{
			"update_id": 801,
			"channel_post": map[string]any{
				"message_id": 101,
				"date":       msgDate.Unix(),
				"text":       "Still works",
				"chat":       map[string]any{"id": -100, "type": "channel"},
			},
		},
	})

	client, err := NewTelegramClient("test-token", "-100", "tg", server.URL, nil, nil, "https://cdn.example.com")
	require.NoError(t, err)
	defer client.Close()

	posts := waitForPosts(t, client, 2, 3*time.Second)
	require.Len(t, posts, 2)

	assert.Equal(t, "Broken photo", posts[0].Content)
	assert.Empty(t, posts[0].Media, "failed download should not block the post, just skip the media")
	assert.Equal(t, "Still works", posts[1].Content)
}

func TestTelegram_Post_NotImplemented(t *testing.T) {
	server := newFakeTelegramServer(t)
	client, err := NewTelegramClient("test-token", "chan", "tg", server.URL, nil, nil, "")
	require.NoError(t, err)
	defer client.Close()

	result, err := client.Post(context.Background(), &Post{Content: "hello"})
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestInitSocialPlatforms_Telegram(t *testing.T) {
	configs := map[string]*PlatformConfig{
		"my-tg": {
			Type:    "telegram",
			Enabled: true,
			Telegram: &TelegramConfig{
				BotToken:  "123:ABC",
				ChannelID: "-1001234567890",
			},
		},
	}

	platforms, err := InitSocialPlatforms(configs, nil, nil, nil, "")
	require.NoError(t, err)
	require.Len(t, platforms, 1)

	p := platforms[0]
	assert.Equal(t, "my-tg", p.Name)
	assert.Equal(t, "my-tg", p.Client.Name())

	// Verify it's actually a TelegramClient, then stop its background
	// long-poll loop (it would otherwise hit the real Telegram API).
	tgClient, ok := p.Client.(*TelegramClient)
	require.True(t, ok, "client should be *TelegramClient")
	tgClient.Close()
}

func TestInitSocialPlatforms_Telegram_MissingConfig(t *testing.T) {
	configs := map[string]*PlatformConfig{
		"bad-tg": {
			Type:    "telegram",
			Enabled: true,
		},
	}

	_, err := InitSocialPlatforms(configs, nil, nil, nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing Telegram config")
}

func TestInitSocialPlatforms_Telegram_MissingCredentials(t *testing.T) {
	configs := map[string]*PlatformConfig{
		"bad-tg": {
			Type:    "telegram",
			Enabled: true,
			Telegram: &TelegramConfig{
				BotToken: "123:ABC",
				// ChannelID missing
			},
		},
	}

	_, err := InitSocialPlatforms(configs, nil, nil, nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing Telegram credentials")
}
