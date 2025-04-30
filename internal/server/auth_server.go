package server

import (
	"context"

	"go.orx.me/apps/hyper-sync/internal/service"
	pb "go.orx.me/apps/hyper-sync/pkg/proto/api/v1"
)

// AuthServer implements the gRPC AuthService server interface
type AuthServer struct {
	pb.UnimplementedAuthServiceServer
	authService *service.AuthService
}

// NewAuthServer creates a new instance of AuthServer
func NewAuthServer(authService *service.AuthService) *AuthServer {
	return &AuthServer{
		authService: authService,
	}
}

// Register implements the Register RPC method
func (s *AuthServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	return s.authService.Register(ctx, req)
}

// Login implements the Login RPC method
func (s *AuthServer) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	return s.authService.Login(ctx, req)
}

// UpdateProfile implements the UpdateProfile RPC method
func (s *AuthServer) UpdateProfile(ctx context.Context, req *pb.UpdateProfileRequest) (*pb.UpdateProfileResponse, error) {
	return s.authService.UpdateProfile(ctx, req)
}

// LoginWithGoogle implements the LoginWithGoogle RPC method
func (s *AuthServer) LoginWithGoogle(ctx context.Context, req *pb.LoginWithGoogleRequest) (*pb.LoginWithGoogleResponse, error) {
	return s.authService.LoginWithGoogle(ctx, req)
}

// GetMe implements the GetMe RPC method
func (s *AuthServer) GetMe(ctx context.Context, req *pb.GetMeRequest) (*pb.GetMeResponse, error) {
	return s.authService.GetMe(ctx, req)
}
