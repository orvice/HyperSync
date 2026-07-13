package service_test

import (
	"context"
	"errors"
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

// testJWTSecret is the single secret shared by every helper and hand-minted
// token in this file, so a change here can't silently desync the two.
const testJWTSecret = "test-secret-key"

func setupAuthTest(t *testing.T, users ...*auth.User) (v1connect.AuthServiceClient, func()) {
	t.Helper()

	store := auth.NewMemoryUserStore()
	for _, u := range users {
		err := store.Create(context.Background(), u)
		require.NoError(t, err)
	}

	svc := service.NewAuthService(store, testJWTSecret)

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

	return newProtectedClient(t, store)
}

// newProtectedClient mounts the AuthService behind the real interceptor for the
// given store, so tests can inject a store that simulates a database outage.
func newProtectedClient(t *testing.T, store auth.UserStore) (v1connect.AuthServiceClient, func()) {
	t.Helper()

	svc := service.NewAuthService(store, testJWTSecret)
	interceptor := auth.NewAuthInterceptor(testJWTSecret, store)

	mux := http.NewServeMux()
	path, handler := v1connect.NewAuthServiceHandler(svc, connect.WithInterceptors(interceptor))
	mux.Handle(path, handler)

	server := httptest.NewServer(mux)
	client := v1connect.NewAuthServiceClient(server.Client(), server.URL)

	return client, server.Close
}

// outageStore fails every read to simulate a database outage during token
// validation.
type outageStore struct{}

func (outageStore) GetByUsername(context.Context, string) (*auth.User, error) {
	return nil, errors.New("mongo: connection refused")
}
func (outageStore) Create(context.Context, *auth.User) error             { return nil }
func (outageStore) UpdatePassword(context.Context, string, string) error { return nil }

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
	legacyToken, err := legacy.SignedString([]byte(testJWTSecret))
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

func TestProtectedEndpoint_StoreOutage_ReturnsUnavailableNotUnauthenticated(t *testing.T) {
	// A structurally valid token whose validation can't complete because the
	// store is down must surface as Unavailable, not Unauthenticated — else a
	// transient outage looks like revocation and logs every client out.
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":                  "admin",
		"exp":                  time.Now().Add(time.Hour).Unix(),
		auth.TokenVersionClaim: int64(0),
	})
	token, err := tok.SignedString([]byte(testJWTSecret))
	require.NoError(t, err)

	client, cleanup := newProtectedClient(t, outageStore{})
	defer cleanup()

	req := connect.NewRequest(&v1.ChangePasswordRequest{
		CurrentPassword: "whatever",
		NewPassword:     "whatever-new-123",
	})
	req.Header().Set("Authorization", "Bearer "+token)
	_, err = client.ChangePassword(context.Background(), req)

	require.Error(t, err)
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
}
