package tests

import (
	"context"
	"testing"
	"time"

	"go.orx.me/apps/hyper-sync/internal/social"
)

// MockConfigDao for testing token refresh functionality
type MockConfigDao struct {
	tokenInfo     *social.TokenInfo
	shouldFail    bool
	saveCallCount int
}

func NewMockConfigDao(token string, expiresAt *time.Time) *MockConfigDao {
	return &MockConfigDao{
		tokenInfo: &social.TokenInfo{
			AccessToken: token,
			ExpiresAt:   expiresAt,
		},
	}
}

func (m *MockConfigDao) GetAccessToken(ctx context.Context, platform string) (string, error) {
	if m.tokenInfo == nil {
		return "", nil
	}
	return m.tokenInfo.AccessToken, nil
}

func (m *MockConfigDao) GetTokenInfo(ctx context.Context, platform string) (*social.TokenInfo, error) {
	if m.shouldFail {
		return nil, &testError{"failed to get token info"}
	}
	return m.tokenInfo, nil
}

func (m *MockConfigDao) SaveAccessToken(ctx context.Context, platform, accessToken string, expiresAt *time.Time) error {
	m.saveCallCount++
	if m.shouldFail {
		return &testError{"failed to save token"}
	}

	// Update the mock's token info
	m.tokenInfo = &social.TokenInfo{
		AccessToken: accessToken,
		ExpiresAt:   expiresAt,
	}
	return nil
}

type testError struct {
	message string
}

func (e *testError) Error() string {
	return e.message
}

func TestEnsureValidToken_ValidToken(t *testing.T) {
	// Token that expires in 30 days (not soon)
	futureExpiry := time.Now().Add(30 * 24 * time.Hour)
	mockDao := NewMockConfigDao("valid_token", &futureExpiry)

	client := &social.ThreadsClient{}
	client.SetConfigDao(mockDao) // We need to add this method

	err := client.EnsureValidToken(context.Background())
	if err != nil {
		t.Errorf("Expected no error for valid token, got: %v", err)
	}

	// Should not have called save (no refresh needed)
	if mockDao.saveCallCount != 0 {
		t.Errorf("Expected no save calls for valid token, got %d", mockDao.saveCallCount)
	}
}

func TestEnsureValidToken_ExpiringSoon(t *testing.T) {
	// Token that expires in 3 days (should refresh)
	expiringSoon := time.Now().Add(3 * 24 * time.Hour)
	mockDao := NewMockConfigDao("expiring_token", &expiringSoon)

	client := &social.ThreadsClient{}
	client.SetConfigDao(mockDao)

	// This will try to refresh but fail with mock credentials
	// However, since the token is still valid, the method should succeed
	err := client.EnsureValidToken(context.Background())

	// The token is still valid for 3 days, so even if refresh fails,
	// EnsureValidToken should not return an error
	if err != nil {
		t.Errorf("Expected no error since token is still valid, got: %v", err)
	}
}

func TestEnsureValidToken_NoExpiryInfo(t *testing.T) {
	// Token without expiry information
	mockDao := NewMockConfigDao("token_no_expiry", nil)

	client := &social.ThreadsClient{}
	client.SetConfigDao(mockDao)

	err := client.EnsureValidToken(context.Background())
	if err != nil {
		t.Errorf("Expected no error for token without expiry info, got: %v", err)
	}

	// Should not have called save (no refresh needed)
	if mockDao.saveCallCount != 0 {
		t.Errorf("Expected no save calls for token without expiry, got %d", mockDao.saveCallCount)
	}
}

func TestEnsureValidToken_NoToken(t *testing.T) {
	// No token in database
	mockDao := &MockConfigDao{tokenInfo: nil}

	client := &social.ThreadsClient{}
	client.SetConfigDao(mockDao)

	err := client.EnsureValidToken(context.Background())
	if err == nil {
		t.Error("Expected error when no token is found")
	}

	expectedError := "access token not found in database"
	if err != nil && err.Error() != expectedError {
		t.Errorf("Expected error message '%s', got: %v", expectedError, err.Error())
	}
}

// Token refresh related validation tests are part of this file's focus
