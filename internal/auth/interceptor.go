package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type usernameKey struct{}

func UsernameFromContext(ctx context.Context) string {
	v, _ := ctx.Value(usernameKey{}).(string)
	return v
}

var publicProcedures = map[string]bool{
	"/api.v1.AuthService/Login": true,
}

var errInvalidToken = errors.New("invalid token")

// ValidateBearer verifies an "Authorization: Bearer <jwt>" header value and
// returns the subject username. Shared by the Connect interceptor and the
// plain-HTTP middleware so both enforce identical rules. The token's version
// claim must match the user's current TokenVersion, so a password change
// invalidates every previously issued token; tokens minted before versioning
// existed carry an implicit version 0.
func ValidateBearer(ctx context.Context, jwtSecret, authHeader string, store UserStore) (string, error) {
	if authHeader == "" {
		return "", errInvalidToken
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		return "", errInvalidToken
	}

	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(jwtSecret), nil
	}, jwt.WithValidMethods([]string{"HS256", "HS384", "HS512"}), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return "", errInvalidToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errInvalidToken
	}

	username, _ := claims["sub"].(string)

	user, err := store.GetByUsername(ctx, username)
	if err != nil {
		return "", errInvalidToken
	}
	ver, _ := claims["ver"].(float64) // JSON numbers decode as float64; absent → 0
	if int64(ver) != user.TokenVersion {
		return "", errInvalidToken
	}

	return username, nil
}

func NewAuthInterceptor(jwtSecret string, store UserStore) connect.UnaryInterceptorFunc {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if publicProcedures[req.Spec().Procedure] {
				return next(ctx, req)
			}

			username, err := ValidateBearer(ctx, jwtSecret, req.Header().Get("Authorization"), store)
			if err != nil {
				return nil, connect.NewError(connect.CodeUnauthenticated, err)
			}

			ctx = context.WithValue(ctx, usernameKey{}, username)
			return next(ctx, req)
		})
	})
}

// GinMiddleware protects plain HTTP routes (media upload, token management)
// with the same JWT the Connect services use.
func GinMiddleware(jwtSecret string, store UserStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		username, err := ValidateBearer(c.Request.Context(), jwtSecret, c.GetHeader("Authorization"), store)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
			return
		}
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), usernameKey{}, username))
		c.Next()
	}
}
