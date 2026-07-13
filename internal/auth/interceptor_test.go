package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.orx.me/apps/hyper-sync/internal/auth"
)

const testSecret = "interceptor-test-secret"

// fakeStore lets a test choose the error GetByUsername returns, so we can
// distinguish "user missing" from "store down" at the validation boundary.
type fakeStore struct {
	user *auth.User
	err  error
}

func (s *fakeStore) GetByUsername(context.Context, string) (*auth.User, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.user, nil
}
func (s *fakeStore) Create(context.Context, *auth.User) error             { return nil }
func (s *fakeStore) UpdatePassword(context.Context, string, string) error { return nil }

func mintToken(t *testing.T, version int64) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":                  "admin",
		"exp":                  time.Now().Add(time.Hour).Unix(),
		auth.TokenVersionClaim: version,
	})
	signed, err := tok.SignedString([]byte(testSecret))
	require.NoError(t, err)
	return signed
}

func ginStatusFor(t *testing.T, store auth.UserStore, authHeader string) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/protected", auth.GinMiddleware(testSecret, store), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestGinMiddleware_ValidToken_Passes(t *testing.T) {
	store := &fakeStore{user: &auth.User{Username: "admin", TokenVersion: 2}}
	code := ginStatusFor(t, store, "Bearer "+mintToken(t, 2))
	assert.Equal(t, http.StatusOK, code)
}

func TestGinMiddleware_StaleTokenVersion_Rejected(t *testing.T) {
	// User has moved to version 3 (e.g. after a password change); a token
	// minted at version 2 must be rejected on the gin surface too.
	store := &fakeStore{user: &auth.User{Username: "admin", TokenVersion: 3}}
	code := ginStatusFor(t, store, "Bearer "+mintToken(t, 2))
	assert.Equal(t, http.StatusUnauthorized, code)
}

func TestGinMiddleware_StoreOutage_ReturnsServiceUnavailable(t *testing.T) {
	// A transient store failure must not masquerade as revocation (401),
	// which would force every client to log out.
	store := &fakeStore{err: errors.New("mongo: connection refused")}
	code := ginStatusFor(t, store, "Bearer "+mintToken(t, 0))
	assert.Equal(t, http.StatusServiceUnavailable, code)
}

func TestGinMiddleware_MissingUser_ReturnsUnauthorized(t *testing.T) {
	store := &fakeStore{err: auth.ErrUserNotFound}
	code := ginStatusFor(t, store, "Bearer "+mintToken(t, 0))
	assert.Equal(t, http.StatusUnauthorized, code)
}
