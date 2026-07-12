package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	v1 "go.orx.me/apps/hyper-sync/pkg/proto/api/v1"
	"go.orx.me/apps/hyper-sync/pkg/proto/api/v1/v1connect"

	"go.orx.me/apps/hyper-sync/internal/auth"
	"go.orx.me/apps/hyper-sync/internal/service"
)

func setupAuthTest(t *testing.T, users ...*auth.User) (v1connect.AuthServiceClient, func()) {
	t.Helper()

	store := auth.NewMemoryUserStore()
	for _, u := range users {
		err := store.Create(context.Background(), u)
		require.NoError(t, err)
	}

	jwtSecret := "test-secret-key"
	svc := service.NewAuthService(store, jwtSecret)

	mux := http.NewServeMux()
	path, handler := v1connect.NewAuthServiceHandler(svc)
	mux.Handle(path, handler)

	server := httptest.NewServer(mux)
	client := v1connect.NewAuthServiceClient(server.Client(), server.URL)

	return client, server.Close
}

func hashPassword(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err)
	return string(hash)
}

func TestLogin_ValidCredentials_ReturnsToken(t *testing.T) {
	user := &auth.User{
		Username:     "admin",
		PasswordHash: hashPassword(t, "correct-password"),
	}

	client, cleanup := setupAuthTest(t, user)
	defer cleanup()

	resp, err := client.Login(context.Background(), connect.NewRequest(&v1.LoginRequest{
		Username: "admin",
		Password: "correct-password",
	}))

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Msg.Token)
	assert.Greater(t, resp.Msg.ExpiresAt, int64(0))
}

func TestLogin_WrongPassword_ReturnsUnauthenticated(t *testing.T) {
	user := &auth.User{
		Username:     "admin",
		PasswordHash: hashPassword(t, "correct-password"),
	}

	client, cleanup := setupAuthTest(t, user)
	defer cleanup()

	_, err := client.Login(context.Background(), connect.NewRequest(&v1.LoginRequest{
		Username: "admin",
		Password: "wrong-password",
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestLogin_NonExistentUser_ReturnsUnauthenticated(t *testing.T) {
	client, cleanup := setupAuthTest(t)
	defer cleanup()

	_, err := client.Login(context.Background(), connect.NewRequest(&v1.LoginRequest{
		Username: "nobody",
		Password: "some-password",
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func setupProtectedAuthTest(t *testing.T, users ...*auth.User) (v1connect.AuthServiceClient, func()) {
	t.Helper()

	store := auth.NewMemoryUserStore()
	for _, u := range users {
		err := store.Create(context.Background(), u)
		require.NoError(t, err)
	}

	jwtSecret := "test-secret-key"
	svc := service.NewAuthService(store, jwtSecret)
	interceptor := auth.NewAuthInterceptor(jwtSecret, store)

	mux := http.NewServeMux()
	path, handler := v1connect.NewAuthServiceHandler(svc, connect.WithInterceptors(interceptor))
	mux.Handle(path, handler)

	server := httptest.NewServer(mux)
	client := v1connect.NewAuthServiceClient(server.Client(), server.URL)

	return client, server.Close
}

func TestProtectedEndpoint_NoToken_ReturnsUnauthenticated(t *testing.T) {
	user := &auth.User{
		Username:     "admin",
		PasswordHash: hashPassword(t, "password"),
	}
	client, cleanup := setupProtectedAuthTest(t, user)
	defer cleanup()

	_, err := client.ChangePassword(context.Background(), connect.NewRequest(&v1.ChangePasswordRequest{
		CurrentPassword: "password",
		NewPassword:     "newpass",
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestProtectedEndpoint_InvalidToken_ReturnsUnauthenticated(t *testing.T) {
	user := &auth.User{
		Username:     "admin",
		PasswordHash: hashPassword(t, "password"),
	}
	client, cleanup := setupProtectedAuthTest(t, user)
	defer cleanup()

	req := connect.NewRequest(&v1.ChangePasswordRequest{
		CurrentPassword: "password",
		NewPassword:     "newpass",
	})
	req.Header().Set("Authorization", "Bearer invalid-token-garbage")

	_, err := client.ChangePassword(context.Background(), req)

	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestProtectedEndpoint_ValidToken_Succeeds(t *testing.T) {
	user := &auth.User{
		Username:     "admin",
		PasswordHash: hashPassword(t, "password"),
	}
	client, cleanup := setupProtectedAuthTest(t, user)
	defer cleanup()

	// First login to get a valid token
	loginResp, err := client.Login(context.Background(), connect.NewRequest(&v1.LoginRequest{
		Username: "admin",
		Password: "password",
	}))
	require.NoError(t, err)

	// Use the token to call a protected endpoint
	req := connect.NewRequest(&v1.ChangePasswordRequest{
		CurrentPassword: "password",
		NewPassword:     "newpassword",
	})
	req.Header().Set("Authorization", "Bearer "+loginResp.Msg.Token)

	_, err = client.ChangePassword(context.Background(), req)

	require.NoError(t, err)
}

func TestChangePassword_InvalidatesOutstandingTokens(t *testing.T) {
	user := &auth.User{
		Username:     "admin",
		PasswordHash: hashPassword(t, "old-password"),
	}
	client, cleanup := setupProtectedAuthTest(t, user)
	defer cleanup()

	loginResp, err := client.Login(context.Background(), connect.NewRequest(&v1.LoginRequest{
		Username: "admin",
		Password: "old-password",
	}))
	require.NoError(t, err)
	oldToken := loginResp.Msg.Token

	req := connect.NewRequest(&v1.ChangePasswordRequest{
		CurrentPassword: "old-password",
		NewPassword:     "new-password-123",
	})
	req.Header().Set("Authorization", "Bearer "+oldToken)
	_, err = client.ChangePassword(context.Background(), req)
	require.NoError(t, err)

	// The pre-change token must be rejected immediately, including the caller's own.
	req2 := connect.NewRequest(&v1.ChangePasswordRequest{
		CurrentPassword: "new-password-123",
		NewPassword:     "another-password-456",
	})
	req2.Header().Set("Authorization", "Bearer "+oldToken)
	_, err = client.ChangePassword(context.Background(), req2)

	require.Error(t, err, "token issued before the password change must be rejected")
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestChangePassword_NewLoginIssuesWorkingToken(t *testing.T) {
	user := &auth.User{
		Username:     "admin",
		PasswordHash: hashPassword(t, "old-password"),
	}
	client, cleanup := setupProtectedAuthTest(t, user)
	defer cleanup()

	loginResp, err := client.Login(context.Background(), connect.NewRequest(&v1.LoginRequest{
		Username: "admin",
		Password: "old-password",
	}))
	require.NoError(t, err)

	req := connect.NewRequest(&v1.ChangePasswordRequest{
		CurrentPassword: "old-password",
		NewPassword:     "new-password-123",
	})
	req.Header().Set("Authorization", "Bearer "+loginResp.Msg.Token)
	_, err = client.ChangePassword(context.Background(), req)
	require.NoError(t, err)

	// Re-login with the new password; the fresh token must be accepted.
	relogin, err := client.Login(context.Background(), connect.NewRequest(&v1.LoginRequest{
		Username: "admin",
		Password: "new-password-123",
	}))
	require.NoError(t, err)

	req2 := connect.NewRequest(&v1.ChangePasswordRequest{
		CurrentPassword: "new-password-123",
		NewPassword:     "another-password-456",
	})
	req2.Header().Set("Authorization", "Bearer "+relogin.Msg.Token)
	_, err = client.ChangePassword(context.Background(), req2)
	require.NoError(t, err, "token from a post-change login must be accepted")
}

func TestChangePassword_OtherUsersTokensUnaffected(t *testing.T) {
	alice := &auth.User{Username: "alice", PasswordHash: hashPassword(t, "alice-password")}
	bob := &auth.User{Username: "bob", PasswordHash: hashPassword(t, "bob-password")}
	client, cleanup := setupProtectedAuthTest(t, alice, bob)
	defer cleanup()

	bobLogin, err := client.Login(context.Background(), connect.NewRequest(&v1.LoginRequest{
		Username: "bob",
		Password: "bob-password",
	}))
	require.NoError(t, err)

	aliceLogin, err := client.Login(context.Background(), connect.NewRequest(&v1.LoginRequest{
		Username: "alice",
		Password: "alice-password",
	}))
	require.NoError(t, err)

	req := connect.NewRequest(&v1.ChangePasswordRequest{
		CurrentPassword: "alice-password",
		NewPassword:     "alice-new-password",
	})
	req.Header().Set("Authorization", "Bearer "+aliceLogin.Msg.Token)
	_, err = client.ChangePassword(context.Background(), req)
	require.NoError(t, err)

	// Bob's token predates Alice's change but belongs to an unchanged user.
	req2 := connect.NewRequest(&v1.ChangePasswordRequest{
		CurrentPassword: "bob-password",
		NewPassword:     "bob-new-password",
	})
	req2.Header().Set("Authorization", "Bearer "+bobLogin.Msg.Token)
	_, err = client.ChangePassword(context.Background(), req2)
	require.NoError(t, err, "an unchanged user's token must stay valid")
}

func TestLegacyTokenWithoutVersionClaim_StillValidUntilPasswordChange(t *testing.T) {
	user := &auth.User{
		Username:     "admin",
		PasswordHash: hashPassword(t, "password"),
	}
	client, cleanup := setupProtectedAuthTest(t, user)
	defer cleanup()

	// A token minted before versioning existed: no "ver" claim.
	legacy := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "admin",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	legacyToken, err := legacy.SignedString([]byte("test-secret-key"))
	require.NoError(t, err)

	req := connect.NewRequest(&v1.ChangePasswordRequest{
		CurrentPassword: "password",
		NewPassword:     "new-password-123",
	})
	req.Header().Set("Authorization", "Bearer "+legacyToken)
	_, err = client.ChangePassword(context.Background(), req)
	require.NoError(t, err, "pre-versioning token must stay valid for an unchanged user")

	// After the change it must be rejected like any other stale token.
	req2 := connect.NewRequest(&v1.ChangePasswordRequest{
		CurrentPassword: "new-password-123",
		NewPassword:     "another-password-456",
	})
	req2.Header().Set("Authorization", "Bearer "+legacyToken)
	_, err = client.ChangePassword(context.Background(), req2)
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}
