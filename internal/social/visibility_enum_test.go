package social

import (
	"testing"
)

func TestVisibilityLevel_String(t *testing.T) {
	tests := []struct {
		name     string
		level    VisibilityLevel
		expected string
	}{
		{
			name:     "public level",
			level:    VisibilityLevelPublic,
			expected: "public",
		},
		{
			name:     "unlisted level",
			level:    VisibilityLevelUnlisted,
			expected: "unlisted",
		},
		{
			name:     "private level",
			level:    VisibilityLevelPrivate,
			expected: "private",
		},
		{
			name:     "direct level",
			level:    VisibilityLevelDirect,
			expected: "direct",
		},
		{
			name:     "invalid level",
			level:    VisibilityLevel(99),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.level.String()
			if result != tt.expected {
				t.Errorf("expected %s but got %s", tt.expected, result)
			}
		})
	}
}

func TestVisibilityLevel_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		level    VisibilityLevel
		expected bool
	}{
		{
			name:     "public is valid",
			level:    VisibilityLevelPublic,
			expected: true,
		},
		{
			name:     "unlisted is valid",
			level:    VisibilityLevelUnlisted,
			expected: true,
		},
		{
			name:     "private is valid",
			level:    VisibilityLevelPrivate,
			expected: true,
		},
		{
			name:     "direct is valid",
			level:    VisibilityLevelDirect,
			expected: true,
		},
		{
			name:     "negative level is invalid",
			level:    VisibilityLevel(-1),
			expected: false,
		},
		{
			name:     "too high level is invalid",
			level:    VisibilityLevel(99),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.level.IsValid()
			if result != tt.expected {
				t.Errorf("expected %v but got %v", tt.expected, result)
			}
		})
	}
}

func TestParseVisibilityLevel(t *testing.T) {
	tests := []struct {
		name        string
		visibility  string
		expected    VisibilityLevel
		expectError bool
	}{
		{
			name:        "parse public",
			visibility:  "public",
			expected:    VisibilityLevelPublic,
			expectError: false,
		},
		{
			name:        "parse unlisted",
			visibility:  "unlisted",
			expected:    VisibilityLevelUnlisted,
			expectError: false,
		},
		{
			name:        "parse private",
			visibility:  "private",
			expected:    VisibilityLevelPrivate,
			expectError: false,
		},
		{
			name:        "parse direct",
			visibility:  "direct",
			expected:    VisibilityLevelDirect,
			expectError: false,
		},
		{
			name:        "parse invalid",
			visibility:  "invalid",
			expected:    VisibilityLevel(-1),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseVisibilityLevel(tt.visibility)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError && result != tt.expected {
				t.Errorf("expected %v but got %v", tt.expected, result)
			}
		})
	}
}

func TestParsePlatformVisibility(t *testing.T) {
	tests := []struct {
		name        string
		platform    string
		visibility  string
		expected    VisibilityLevel
		expectError bool
	}{
		// Memos specific tests
		{
			name:        "memos PUBLIC",
			platform:    "memos",
			visibility:  MemosVisibilityPublic,
			expected:    VisibilityLevelPublic,
			expectError: false,
		},
		{
			name:        "memos PROTECTED",
			platform:    "memos",
			visibility:  MemosVisibilityProtected,
			expected:    VisibilityLevelUnlisted,
			expectError: false,
		},
		{
			name:        "memos PRIVATE",
			platform:    "memos",
			visibility:  MemosVisibilityPrivate,
			expected:    VisibilityLevelPrivate,
			expectError: false,
		},
		{
			name:        "memos standard value",
			platform:    "memos",
			visibility:  "public",
			expected:    VisibilityLevelPublic,
			expectError: false,
		},

		// Other platforms
		{
			name:        "mastodon public",
			platform:    "mastodon",
			visibility:  "public",
			expected:    VisibilityLevelPublic,
			expectError: false,
		},
		{
			name:        "bluesky private",
			platform:    "bluesky",
			visibility:  "private",
			expected:    VisibilityLevelPrivate,
			expectError: false,
		},
		{
			name:        "invalid visibility",
			platform:    "mastodon",
			visibility:  "invalid",
			expected:    VisibilityLevel(-1),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParsePlatformVisibility(tt.platform, tt.visibility)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError && result != tt.expected {
				t.Errorf("expected %v but got %v", tt.expected, result)
			}
		})
	}
}

func TestValidateVisibilityLevel(t *testing.T) {
	tests := []struct {
		name        string
		platform    string
		level       VisibilityLevel
		expectError bool
	}{
		// Mastodon tests
		{
			name:        "mastodon public valid",
			platform:    "mastodon",
			level:       VisibilityLevelPublic,
			expectError: false,
		},
		{
			name:        "mastodon direct valid",
			platform:    "mastodon",
			level:       VisibilityLevelDirect,
			expectError: false,
		},

		// Bluesky tests
		{
			name:        "bluesky public valid",
			platform:    "bluesky",
			level:       VisibilityLevelPublic,
			expectError: false,
		},
		{
			name:        "bluesky direct invalid",
			platform:    "bluesky",
			level:       VisibilityLevelDirect,
			expectError: true,
		},

		// Invalid level
		{
			name:        "invalid level",
			platform:    "mastodon",
			level:       VisibilityLevel(-1),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVisibilityLevel(tt.platform, tt.level)
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestGetPlatformVisibilityString(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		level    VisibilityLevel
		expected string
	}{
		// Memos platform
		{
			name:     "memos public",
			platform: "memos",
			level:    VisibilityLevelPublic,
			expected: MemosVisibilityPublic,
		},
		{
			name:     "memos unlisted",
			platform: "memos",
			level:    VisibilityLevelUnlisted,
			expected: MemosVisibilityProtected,
		},
		{
			name:     "memos private",
			platform: "memos",
			level:    VisibilityLevelPrivate,
			expected: MemosVisibilityPrivate,
		},

		// Other platforms
		{
			name:     "mastodon public",
			platform: "mastodon",
			level:    VisibilityLevelPublic,
			expected: "public",
		},
		{
			name:     "bluesky private",
			platform: "bluesky",
			level:    VisibilityLevelPrivate,
			expected: "private",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPlatformVisibilityString(tt.platform, tt.level)
			if result != tt.expected {
				t.Errorf("expected %s but got %s", tt.expected, result)
			}
		})
	}
}

func TestValidateAndNormalizeVisibilityLevel(t *testing.T) {
	tests := []struct {
		name        string
		platform    string
		visibility  string
		expected    VisibilityLevel
		expectError bool
	}{
		// Valid cases
		{
			name:        "memos PUBLIC normalized",
			platform:    "memos",
			visibility:  MemosVisibilityPublic,
			expected:    VisibilityLevelPublic,
			expectError: false,
		},
		{
			name:        "mastodon public",
			platform:    "mastodon",
			visibility:  "public",
			expected:    VisibilityLevelPublic,
			expectError: false,
		},
		{
			name:        "empty visibility gets default",
			platform:    "bluesky",
			visibility:  "",
			expected:    VisibilityLevelPublic,
			expectError: false,
		},

		// Invalid cases
		{
			name:        "invalid visibility for platform",
			platform:    "bluesky",
			visibility:  "direct",
			expected:    VisibilityLevel(-1),
			expectError: true,
		},
		{
			name:        "unknown visibility value",
			platform:    "mastodon",
			visibility:  "invalid",
			expected:    VisibilityLevel(-1),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateAndNormalizeVisibilityLevel(tt.platform, tt.visibility)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError && result != tt.expected {
				t.Errorf("expected %v but got %v", tt.expected, result)
			}
		})
	}
}
