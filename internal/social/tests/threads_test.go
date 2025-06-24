package tests

import (
	"testing"
	"time"

	"go.orx.me/apps/hyper-sync/internal/social"
)

func TestNewThreadsClient(t *testing.T) {
	client, err := social.NewThreadsClient("test_client_id", "test_client_secret", "test_access_token")
	if err != nil {
		t.Fatalf("Failed to create Threads client: %v", err)
	}

	if client.ClientID != "test_client_id" {
		t.Errorf("Expected ClientID to be 'test_client_id', got '%s'", client.ClientID)
	}

	if client.ClientSecret != "test_client_secret" {
		t.Errorf("Expected ClientSecret to be 'test_client_secret', got '%s'", client.ClientSecret)
	}

	if client.AccessToken != "test_access_token" {
		t.Errorf("Expected AccessToken to be 'test_access_token', got '%s'", client.AccessToken)
	}
}

func TestExchangeForLongLivedToken_MissingClientSecret(t *testing.T) {
	client, _ := social.NewThreadsClient("test_client_id", "", "test_access_token")

	_, err := client.ExchangeForLongLivedToken("short_lived_token")
	if err == nil {
		t.Error("Expected error when client secret is missing")
	}

	expectedError := "client secret is required for token exchange"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestRefreshLongLivedToken_MissingAccessToken(t *testing.T) {
	client, _ := social.NewThreadsClient("test_client_id", "test_client_secret", "")

	_, err := client.RefreshLongLivedToken()
	if err == nil {
		t.Error("Expected error when access token is missing")
	}

	expectedError := "access token is required for token refresh"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestTokenResponse_GetTokenExpirationTime(t *testing.T) {
	tokenResp := &social.TokenResponse{
		AccessToken: "test_token",
		TokenType:   "bearer",
		ExpiresIn:   3600, // 1 hour
	}

	expirationTime := tokenResp.GetTokenExpirationTime()

	// 允许1秒的误差
	if time.Until(expirationTime) > time.Hour+time.Second || time.Until(expirationTime) < time.Hour-time.Second {
		t.Errorf("Token expiration time calculation is incorrect")
	}
}

func TestTokenResponse_IsTokenExpiringSoon(t *testing.T) {
	tests := []struct {
		name      string
		expiresIn int64
		threshold time.Duration
		expected  bool
	}{
		{
			name:      "Token expires in 6 days, threshold 7 days",
			expiresIn: 6 * 24 * 3600, // 6 days
			threshold: 7 * 24 * time.Hour,
			expected:  true,
		},
		{
			name:      "Token expires in 8 days, threshold 7 days",
			expiresIn: 8 * 24 * 3600, // 8 days
			threshold: 7 * 24 * time.Hour,
			expected:  false,
		},
		{
			name:      "Token expires in 1 hour, threshold 1 day",
			expiresIn: 3600, // 1 hour
			threshold: 24 * time.Hour,
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenResp := &social.TokenResponse{
				AccessToken: "test_token",
				TokenType:   "bearer",
				ExpiresIn:   tt.expiresIn,
			}

			result := tokenResp.IsTokenExpiringSoon(tt.threshold)
			if result != tt.expected {
				t.Errorf("IsTokenExpiringSoon() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestTokenResponse_ShouldRefreshToken(t *testing.T) {
	tests := []struct {
		name      string
		expiresIn int64
		expected  bool
	}{
		{
			name:      "Token expires in 6 days - should refresh",
			expiresIn: 6 * 24 * 3600, // 6 days
			expected:  true,
		},
		{
			name:      "Token expires in 8 days - should not refresh yet",
			expiresIn: 8 * 24 * 3600, // 8 days
			expected:  false,
		},
		{
			name:      "Token expires in 1 day - should refresh",
			expiresIn: 24 * 3600, // 1 day
			expected:  true,
		},
		{
			name:      "Token expires in 30 days - should not refresh yet",
			expiresIn: 30 * 24 * 3600, // 30 days
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenResp := &social.TokenResponse{
				AccessToken: "test_token",
				TokenType:   "bearer",
				ExpiresIn:   tt.expiresIn,
			}

			result := tokenResp.ShouldRefreshToken()
			if result != tt.expected {
				t.Errorf("ShouldRefreshToken() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestTokenResponse_Fields(t *testing.T) {
	tokenResp := &social.TokenResponse{
		AccessToken: "test_access_token_123",
		TokenType:   "bearer",
		ExpiresIn:   5183944, // 60 days in seconds
	}

	if tokenResp.AccessToken != "test_access_token_123" {
		t.Errorf("Expected AccessToken to be 'test_access_token_123', got '%s'", tokenResp.AccessToken)
	}

	if tokenResp.TokenType != "bearer" {
		t.Errorf("Expected TokenType to be 'bearer', got '%s'", tokenResp.TokenType)
	}

	if tokenResp.ExpiresIn != 5183944 {
		t.Errorf("Expected ExpiresIn to be 5183944, got %d", tokenResp.ExpiresIn)
	}
}
