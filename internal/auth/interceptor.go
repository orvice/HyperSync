package auth

import (
	"context"
	"strings"

	"connectrpc.com/connect"
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

func NewAuthInterceptor(jwtSecret string) connect.UnaryInterceptorFunc {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if publicProcedures[req.Spec().Procedure] {
				return next(ctx, req)
			}

			authHeader := req.Header().Get("Authorization")
			if authHeader == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, nil)
			}

			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			if tokenString == authHeader {
				return nil, connect.NewError(connect.CodeUnauthenticated, nil)
			}

			token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(jwtSecret), nil
			})
			if err != nil || !token.Valid {
				return nil, connect.NewError(connect.CodeUnauthenticated, nil)
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				return nil, connect.NewError(connect.CodeUnauthenticated, nil)
			}

			username, _ := claims["sub"].(string)
			ctx = context.WithValue(ctx, usernameKey{}, username)

			return next(ctx, req)
		})
	})
}
