package social

import (
	"testing"

	"github.com/mattn/go-mastodon"
)

// These cases mirror the exact shapes each client's Post() returns; the worker
// previously only handled the Memos shape, silently breaking update/delete for
// every other platform.
func TestExtractPlatformID(t *testing.T) {
	cases := []struct {
		name   string
		result interface{}
		want   string
	}{
		{
			name:   "memos map[string]string",
			result: map[string]string{"id": "memos/abc123"},
			want:   "memos/abc123",
		},
		{
			name: "bluesky map[string]interface{} uses rkey",
			result: map[string]interface{}{
				"uri":  "at://did:plc:xyz/app.bsky.feed.post/rkey123",
				"cid":  "bafy...",
				"rkey": "rkey123",
			},
			want: "rkey123",
		},
		{
			name:   "mastodon *mastodon.Status",
			result: &mastodon.Status{ID: mastodon.ID("109876")},
			want:   "109876",
		},
		{
			name:   "threads *PublishResponse",
			result: &PublishResponse{ID: "17890"},
			want:   "17890",
		},
		{
			name:   "nil result",
			result: nil,
			want:   "",
		},
		{
			name:   "unknown shape",
			result: struct{ ID string }{ID: "x"},
			want:   "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExtractPlatformID(tc.result); got != tc.want {
				t.Errorf("ExtractPlatformID() = %q, want %q", got, tc.want)
			}
		})
	}
}
