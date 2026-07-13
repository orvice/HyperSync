package service

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"go.orx.me/apps/hyper-sync/internal/auth"
	v1 "go.orx.me/apps/hyper-sync/pkg/proto/api/v1"
)

type AuthService struct {
	userStore auth.UserStore
	jwtSecret string
}

func NewAuthService(userStore auth.UserStore, jwtSecret string) *AuthService {
	return &AuthService{
		userStore: userStore,
		jwtSecret: jwtSecret,
	}
}

// dummyHash keeps the not-found path doing a bcrypt comparison so response
// timing does not reveal whether a username exists.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), bcrypt.DefaultCost)

func (s *AuthService) Login(ctx context.Context, req *connect.Request[v1.LoginRequest]) (*connect.Response[v1.LoginResponse], error) {
	user, err := s.userStore.GetByUsername(ctx, req.Msg.Username)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Msg.Password))
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Msg.Password)); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":                  user.Username,
		"exp":                  expiresAt.Unix(),
		auth.TokenVersionClaim: user.TokenVersion,
	})

	tokenString, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, nil)
	}

	return connect.NewResponse(&v1.LoginResponse{
		Token:     tokenString,
		ExpiresAt: expiresAt.Unix(),
	}), nil
}

func (s *AuthService) ChangePassword(ctx context.Context, req *connect.Request[v1.ChangePasswordRequest]) (*connect.Response[v1.ChangePasswordResponse], error) {
	username := auth.UsernameFromContext(ctx)
	if username == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	user, err := s.userStore.GetByUsername(ctx, username)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Msg.CurrentPassword)); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if len(req.Msg.NewPassword) < 8 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("new password must be at least 8 characters"))
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.Msg.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, nil)
	}

	if err := s.userStore.UpdatePassword(ctx, username, string(newHash)); err != nil {
		return nil, connect.NewError(connect.CodeInternal, nil)
	}

	return connect.NewResponse(&v1.ChangePasswordResponse{}), nil
}
