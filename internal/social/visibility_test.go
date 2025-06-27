package social

import (
	"testing"
)

func TestValidateVisibility(t *testing.T) {
	tests := []struct {
		name        string
		platform    string
		visibility  string
		expectError bool
	}{
		// Mastodon tests
		{
			name:        "mastodon public valid",
			platform:    "mastodon",
			visibility:  VisibilityPublic,
			expectError: false,
		},
		{
			name:        "mastodon unlisted valid",
			platform:    "mastodon",
			visibility:  VisibilityUnlisted,
			expectError: false,
		},
		{
			name:        "mastodon private valid",
			platform:    "mastodon",
			visibility:  VisibilityPrivate,
			expectError: false,
		},
		{
			name:        "mastodon direct valid",
			platform:    "mastodon",
			visibility:  VisibilityDirect,
			expectError: false,
		},
		{
			name:        "mastodon invalid visibility",
			platform:    "mastodon",
			visibility:  "invalid",
			expectError: true,
		},
		{
			name:        "mastodon empty visibility",
			platform:    "mastodon",
			visibility:  "",
			expectError: false,
		},

		// Bluesky tests
		{
			name:        "bluesky public valid",
			platform:    "bluesky",
			visibility:  VisibilityPublic,
			expectError: false,
		},
		{
			name:        "bluesky private valid",
			platform:    "bluesky",
			visibility:  VisibilityPrivate,
			expectError: false,
		},
		{
			name:        "bluesky unlisted invalid",
			platform:    "bluesky",
			visibility:  VisibilityUnlisted,
			expectError: true,
		},
		{
			name:        "bluesky direct invalid",
			platform:    "bluesky",
			visibility:  VisibilityDirect,
			expectError: true,
		},

		// Threads tests
		{
			name:        "threads public valid",
			platform:    "threads",
			visibility:  VisibilityPublic,
			expectError: false,
		},
		{
			name:        "threads private valid",
			platform:    "threads",
			visibility:  VisibilityPrivate,
			expectError: false,
		},
		{
			name:        "threads unlisted invalid",
			platform:    "threads",
			visibility:  VisibilityUnlisted,
			expectError: true,
		},

		// Memos tests
		{
			name:        "memos public valid",
			platform:    "memos",
			visibility:  VisibilityPublic,
			expectError: false,
		},
		{
			name:        "memos unlisted valid",
			platform:    "memos",
			visibility:  VisibilityUnlisted,
			expectError: false,
		},
		{
			name:        "memos private valid",
			platform:    "memos",
			visibility:  VisibilityPrivate,
			expectError: false,
		},
		{
			name:        "memos direct invalid",
			platform:    "memos",
			visibility:  VisibilityDirect,
			expectError: true,
		},

		// Unknown platform tests
		{
			name:        "unknown platform default values",
			platform:    "unknown",
			visibility:  VisibilityPublic,
			expectError: false,
		},
		{
			name:        "unknown platform invalid",
			platform:    "unknown",
			visibility:  "invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVisibility(tt.platform, tt.visibility)
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestNormalizeVisibility(t *testing.T) {
	tests := []struct {
		name       string
		platform   string
		visibility string
		expected   string
	}{
		// Memos normalization tests
		{
			name:       "memos PUBLIC to public",
			platform:   "memos",
			visibility: MemosVisibilityPublic,
			expected:   VisibilityPublic,
		},
		{
			name:       "memos PROTECTED to unlisted",
			platform:   "memos",
			visibility: MemosVisibilityProtected,
			expected:   VisibilityUnlisted,
		},
		{
			name:       "memos PRIVATE to private",
			platform:   "memos",
			visibility: MemosVisibilityPrivate,
			expected:   VisibilityPrivate,
		},
		{
			name:       "memos empty to default",
			platform:   "memos",
			visibility: "",
			expected:   VisibilityPublic,
		},

		// Other platforms should return as-is
		{
			name:       "mastodon public unchanged",
			platform:   "mastodon",
			visibility: VisibilityPublic,
			expected:   VisibilityPublic,
		},
		{
			name:       "bluesky empty to default",
			platform:   "bluesky",
			visibility: "",
			expected:   VisibilityPublic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeVisibility(tt.platform, tt.visibility)
			if result != tt.expected {
				t.Errorf("expected %s but got %s", tt.expected, result)
			}
		})
	}
}

func TestGetPlatformVisibility(t *testing.T) {
	tests := []struct {
		name       string
		platform   string
		visibility string
		expected   string
	}{
		// Memos conversion tests
		{
			name:       "memos public to PUBLIC",
			platform:   "memos",
			visibility: VisibilityPublic,
			expected:   MemosVisibilityPublic,
		},
		{
			name:       "memos unlisted to PROTECTED",
			platform:   "memos",
			visibility: VisibilityUnlisted,
			expected:   MemosVisibilityProtected,
		},
		{
			name:       "memos private to PRIVATE",
			platform:   "memos",
			visibility: VisibilityPrivate,
			expected:   MemosVisibilityPrivate,
		},

		// Other platforms should return as-is
		{
			name:       "mastodon public unchanged",
			platform:   "mastodon",
			visibility: VisibilityPublic,
			expected:   VisibilityPublic,
		},
		{
			name:       "bluesky private unchanged",
			platform:   "bluesky",
			visibility: VisibilityPrivate,
			expected:   VisibilityPrivate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPlatformVisibility(tt.platform, tt.visibility)
			if result != tt.expected {
				t.Errorf("expected %s but got %s", tt.expected, result)
			}
		})
	}
}

func TestValidateAndNormalizeVisibility(t *testing.T) {
	tests := []struct {
		name        string
		platform    string
		visibility  string
		expected    string
		expectError bool
	}{
		// Valid cases
		{
			name:        "memos PUBLIC normalized and validated",
			platform:    "memos",
			visibility:  MemosVisibilityPublic,
			expected:    VisibilityPublic,
			expectError: false,
		},
		{
			name:        "mastodon public validated",
			platform:    "mastodon",
			visibility:  VisibilityPublic,
			expected:    VisibilityPublic,
			expectError: false,
		},
		{
			name:        "empty visibility gets default",
			platform:    "bluesky",
			visibility:  "",
			expected:    VisibilityPublic,
			expectError: false,
		},

		// Invalid cases
		{
			name:        "invalid visibility for platform",
			platform:    "bluesky",
			visibility:  VisibilityDirect,
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateAndNormalizeVisibility(tt.platform, tt.visibility)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError && result != tt.expected {
				t.Errorf("expected %s but got %s", tt.expected, result)
			}
		})
	}
}
