package social

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelegram_ListPosts_SingleTextMessage(t *testing.T) {
	msgDate := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottest-token/getUpdates" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}

		resp := map[string]interface{}{
			"ok": true,
			"result": []map[string]interface{}{
				{
					"update_id": 100,
					"channel_post": map[string]interface{}{
						"message_id": 42,
						"date":       msgDate.Unix(),
						"text":       "Hello from Telegram",
						"chat": map[string]interface{}{
							"id":   -1001234567890,
							"type": "channel",
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewTelegramClient("test-token", "-1001234567890", "my-telegram", server.URL, nil)

	posts, err := client.ListPosts(context.Background(), 10)
	require.NoError(t, err)
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bottest-token/getUpdates":
			resp := map[string]interface{}{
				"ok": true,
				"result": []map[string]interface{}{
					{
						"update_id": 200,
						"channel_post": map[string]interface{}{
							"message_id": 55,
							"date":       msgDate.Unix(),
							"caption":    "A beautiful sunset",
							"chat": map[string]interface{}{
								"id":   -1001234567890,
								"type": "channel",
							},
							"photo": []map[string]interface{}{
								{"file_id": "small_id", "file_unique_id": "s1", "width": 90, "height": 90, "file_size": 1000},
								{"file_id": "medium_id", "file_unique_id": "s2", "width": 320, "height": 320, "file_size": 5000},
								{"file_id": "large_id", "file_unique_id": "s3", "width": 800, "height": 800, "file_size": 50000},
							},
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case "/bottest-token/getFile":
			fileID := r.URL.Query().Get("file_id")
			if fileID != "large_id" {
				t.Errorf("expected getFile for large_id, got %s", fileID)
			}
			resp := map[string]interface{}{
				"ok": true,
				"result": map[string]interface{}{
					"file_id":   "large_id",
					"file_path": "photos/file_123.jpg",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		default:
			t.Errorf("unexpected request path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewTelegramClient("test-token", "-1001234567890", "my-telegram", server.URL, nil)

	posts, err := client.ListPosts(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, posts, 1)

	post := posts[0]
	assert.Equal(t, "A beautiful sunset", post.Content)
	assert.Equal(t, "55", post.OriginalID)
	assert.Equal(t, VisibilityLevelPublic, post.Visibility)
	require.Len(t, post.Media, 1)

	expectedURL := server.URL + "/file/bottest-token/photos/file_123.jpg"
	assert.Equal(t, expectedURL, post.Media[0].GetURL())
}

func TestMergeMediaGroups(t *testing.T) {
	tests := []struct {
		name     string
		updates  []tgUpdate
		wantLen  int
		checkFn  func(t *testing.T, merged []tgUpdate)
	}{
		{
			name: "three photos in one group become single update",
			updates: []tgUpdate{
				{UpdateID: 1, ChannelPost: &tgMessage{
					MessageID:    10,
					Date:         1000,
					Caption:      "Album caption",
					MediaGroupID: "g1",
					Photo:        []tgPhotoSize{{FileID: "a1", Width: 800, Height: 600}},
				}},
				{UpdateID: 2, ChannelPost: &tgMessage{
					MessageID:    11,
					Date:         1001,
					MediaGroupID: "g1",
					Photo:        []tgPhotoSize{{FileID: "a2", Width: 800, Height: 600}},
				}},
				{UpdateID: 3, ChannelPost: &tgMessage{
					MessageID:    12,
					Date:         1002,
					MediaGroupID: "g1",
					Photo:        []tgPhotoSize{{FileID: "a3", Width: 800, Height: 600}},
				}},
			},
			wantLen: 1,
			checkFn: func(t *testing.T, merged []tgUpdate) {
				msg := merged[0].ChannelPost
				assert.Equal(t, "Album caption", msg.Caption)
				assert.Len(t, msg.Photo, 3)
				assert.Equal(t, "a1", msg.Photo[0].FileID)
				assert.Equal(t, "a2", msg.Photo[1].FileID)
				assert.Equal(t, "a3", msg.Photo[2].FileID)
				assert.Equal(t, int64(3), merged[0].UpdateID, "merged update should keep max update_id")
			},
		},
		{
			name: "standalone text update unchanged",
			updates: []tgUpdate{
				{UpdateID: 5, ChannelPost: &tgMessage{
					MessageID: 20,
					Date:      2000,
					Text:      "Just text",
				}},
			},
			wantLen: 1,
			checkFn: func(t *testing.T, merged []tgUpdate) {
				assert.Equal(t, "Just text", merged[0].ChannelPost.Text)
				assert.Empty(t, merged[0].ChannelPost.Photo)
			},
		},
		{
			name: "mixed: group + standalone",
			updates: []tgUpdate{
				{UpdateID: 10, ChannelPost: &tgMessage{
					MessageID:    30,
					Date:         3000,
					Caption:      "Group photo",
					MediaGroupID: "g2",
					Photo:        []tgPhotoSize{{FileID: "b1", Width: 800, Height: 600}},
				}},
				{UpdateID: 11, ChannelPost: &tgMessage{
					MessageID: 31,
					Date:      3001,
					Text:      "Standalone text",
				}},
				{UpdateID: 12, ChannelPost: &tgMessage{
					MessageID:    32,
					Date:         3002,
					MediaGroupID: "g2",
					Photo:        []tgPhotoSize{{FileID: "b2", Width: 800, Height: 600}},
				}},
			},
			wantLen: 2,
			checkFn: func(t *testing.T, merged []tgUpdate) {
				// Group comes first in insertion order, standalone second
				group := merged[0]
				standalone := merged[1]
				assert.Equal(t, "Group photo", group.ChannelPost.Caption)
				assert.Len(t, group.ChannelPost.Photo, 2)
				assert.Equal(t, "Standalone text", standalone.ChannelPost.Text)
			},
		},
		{
			name: "caption from first captioned message in group",
			updates: []tgUpdate{
				{UpdateID: 20, ChannelPost: &tgMessage{
					MessageID:    40,
					Date:         4000,
					MediaGroupID: "g3",
					Photo:        []tgPhotoSize{{FileID: "c1", Width: 800, Height: 600}},
				}},
				{UpdateID: 21, ChannelPost: &tgMessage{
					MessageID:    41,
					Date:         4001,
					Caption:      "Caption on second",
					MediaGroupID: "g3",
					Photo:        []tgPhotoSize{{FileID: "c2", Width: 800, Height: 600}},
				}},
			},
			wantLen: 1,
			checkFn: func(t *testing.T, merged []tgUpdate) {
				assert.Equal(t, "Caption on second", merged[0].ChannelPost.Caption)
				assert.Len(t, merged[0].ChannelPost.Photo, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged := mergeMediaGroups(tt.updates)
			require.Len(t, merged, tt.wantLen)
			if tt.checkFn != nil {
				tt.checkFn(t, merged)
			}
		})
	}
}

func TestTelegram_ListPosts_SkippedMessageTypes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"ok": true,
			"result": []map[string]interface{}{
				{
					"update_id": 300,
					"channel_post": map[string]interface{}{
						"message_id": 60,
						"date":       1000,
						"sticker":    map[string]interface{}{"file_id": "sticker1"},
						"chat":       map[string]interface{}{"id": -100, "type": "channel"},
					},
				},
				{
					"update_id": 301,
					"channel_post": map[string]interface{}{
						"message_id": 61,
						"date":       1001,
						"poll":       map[string]interface{}{"question": "yes?"},
						"chat":       map[string]interface{}{"id": -100, "type": "channel"},
					},
				},
				{
					"update_id": 302,
					"channel_post": map[string]interface{}{
						"message_id": 62,
						"date":       1002,
						"document":   map[string]interface{}{"file_id": "doc1"},
						"chat":       map[string]interface{}{"id": -100, "type": "channel"},
					},
				},
				{
					"update_id": 303,
					"channel_post": map[string]interface{}{
						"message_id": 63,
						"date":       1003,
						"audio":      map[string]interface{}{"file_id": "audio1"},
						"chat":       map[string]interface{}{"id": -100, "type": "channel"},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewTelegramClient("test-token", "-100", "tg", server.URL, nil)
	posts, err := client.ListPosts(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, posts, "sticker, poll, document, audio messages should be skipped")
}

// memoryCursor is an in-memory SyncCursorDao for testing.
type memoryCursor struct {
	offsets map[string]int64
}

func newMemoryCursor() *memoryCursor {
	return &memoryCursor{offsets: make(map[string]int64)}
}

func (m *memoryCursor) GetOffset(_ context.Context, platform string) (int64, error) {
	return m.offsets[platform], nil
}

func (m *memoryCursor) SaveOffset(_ context.Context, platform string, offset int64) error {
	m.offsets[platform] = offset
	return nil
}

func TestTelegram_ListPosts_OffsetAdvancement(t *testing.T) {
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottest-token/getUpdates" {
			http.NotFound(w, r)
			return
		}

		offset := r.URL.Query().Get("offset")
		callCount++

		var result []map[string]interface{}
		if callCount == 1 {
			assert.Equal(t, "", offset, "first call should have no offset")
			result = []map[string]interface{}{
				{
					"update_id": 100,
					"channel_post": map[string]interface{}{
						"message_id": 1, "date": 1000, "text": "first",
						"chat": map[string]interface{}{"id": -100, "type": "channel"},
					},
				},
				{
					"update_id": 105,
					"channel_post": map[string]interface{}{
						"message_id": 2, "date": 1001, "text": "second",
						"chat": map[string]interface{}{"id": -100, "type": "channel"},
					},
				},
			}
		} else {
			assert.Equal(t, "106", offset, "second call should send offset=max_update_id+1")
			result = []map[string]interface{}{}
		}

		resp := map[string]interface{}{"ok": true, "result": result}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cursor := newMemoryCursor()
	client := NewTelegramClient("test-token", "-100", "tg", server.URL, cursor)

	// First call: offset should be empty (0), returns 2 posts
	posts, err := client.ListPosts(context.Background(), 10)
	require.NoError(t, err)
	assert.Len(t, posts, 2)

	// Verify offset was saved as max(100, 105) + 1 = 106
	savedOffset, _ := cursor.GetOffset(context.Background(), "tg")
	assert.Equal(t, int64(106), savedOffset)

	// Second call: should send offset=106
	posts, err = client.ListPosts(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, posts)
	assert.Equal(t, 2, callCount)
}

func TestTelegram_ListPosts_EmptyBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{"ok": true, "result": []interface{}{}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cursor := newMemoryCursor()
	cursor.offsets["tg"] = 50 // pre-existing offset

	client := NewTelegramClient("test-token", "-100", "tg", server.URL, cursor)

	posts, err := client.ListPosts(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, posts)

	// Offset should remain unchanged
	savedOffset, _ := cursor.GetOffset(context.Background(), "tg")
	assert.Equal(t, int64(50), savedOffset, "offset should not change on empty batch")
}

func TestTelegram_ListPosts_EntityStripping(t *testing.T) {
	// Telegram's text field is already plain text. Entities like bold/italic
	// are just metadata annotations — the text itself doesn't contain markup.
	// text_link is special: it associates a URL with a text range. We want
	// to append the URL after the link text so it's not lost in plain text.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"ok": true,
			"result": []map[string]interface{}{
				{
					"update_id": 400,
					"channel_post": map[string]interface{}{
						"message_id": 70,
						"date":       1000,
						"text":       "Check this link and bold text here",
						"chat":       map[string]interface{}{"id": -100, "type": "channel"},
						"entities": []map[string]interface{}{
							{"type": "text_link", "offset": 11, "length": 4, "url": "https://example.com"},
							{"type": "bold", "offset": 20, "length": 4},
							{"type": "mention", "offset": 30, "length": 4},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewTelegramClient("test-token", "-100", "tg", server.URL, nil)
	posts, err := client.ListPosts(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, posts, 1)

	// text_link URL should be appended after the link text
	assert.Equal(t, "Check this link (https://example.com) and bold text here", posts[0].Content)
}

func TestTelegram_ListPosts_MediaGroupMerge(t *testing.T) {
	msgDate := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bottest-token/getUpdates":
			resp := map[string]interface{}{
				"ok": true,
				"result": []map[string]interface{}{
					{
						"update_id": 500,
						"channel_post": map[string]interface{}{
							"message_id":     70,
							"date":           msgDate.Unix(),
							"caption":        "Album title",
							"media_group_id": "album1",
							"chat":           map[string]interface{}{"id": -100, "type": "channel"},
							"photo": []map[string]interface{}{
								{"file_id": "p1", "file_unique_id": "u1", "width": 800, "height": 600},
							},
						},
					},
					{
						"update_id": 501,
						"channel_post": map[string]interface{}{
							"message_id":     71,
							"date":           msgDate.Unix(),
							"media_group_id": "album1",
							"chat":           map[string]interface{}{"id": -100, "type": "channel"},
							"photo": []map[string]interface{}{
								{"file_id": "p2", "file_unique_id": "u2", "width": 800, "height": 600},
							},
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case "/bottest-token/getFile":
			fileID := r.URL.Query().Get("file_id")
			resp := map[string]interface{}{
				"ok":     true,
				"result": map[string]interface{}{"file_id": fileID, "file_path": "photos/" + fileID + ".jpg"},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewTelegramClient("test-token", "-100", "tg", server.URL, nil)
	posts, err := client.ListPosts(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, posts, 1, "media group should merge into one post")

	post := posts[0]
	assert.Equal(t, "Album title", post.Content)
	require.Len(t, post.Media, 2)
	assert.Contains(t, post.Media[0].GetURL(), "p1.jpg")
	assert.Contains(t, post.Media[1].GetURL(), "p2.jpg")
}

func TestTelegram_Post_NotImplemented(t *testing.T) {
	client := NewTelegramClient("token", "chan", "tg", "", nil)
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

	platforms, err := InitSocialPlatforms(configs, nil)
	require.NoError(t, err)
	require.Len(t, platforms, 1)

	p := platforms[0]
	assert.Equal(t, "my-tg", p.Name)
	assert.Equal(t, "my-tg", p.Client.Name())

	// Verify it's actually a TelegramClient
	_, ok := p.Client.(*TelegramClient)
	assert.True(t, ok, "client should be *TelegramClient")
}

func TestInitSocialPlatforms_Telegram_MissingConfig(t *testing.T) {
	configs := map[string]*PlatformConfig{
		"bad-tg": {
			Type:    "telegram",
			Enabled: true,
		},
	}

	_, err := InitSocialPlatforms(configs, nil)
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

	_, err := InitSocialPlatforms(configs, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing Telegram credentials")
}
