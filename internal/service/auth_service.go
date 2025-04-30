package service

import (
	"context"
	"errors"

	"time" // Add time package if not already present

	"github.com/golang-jwt/jwt/v5" // Add JWT package
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.orx.me/apps/hyper-sync/internal/dao"        // Import DAO package
	pb "go.orx.me/apps/hyper-sync/pkg/proto/api/v1" // Generated proto package path based on go_package option
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Context key for user ID (replace with your actual implementation)
type contextKey string

const userIDKey contextKey = "userID"

// TODO: Move JWT secret and expiration to configuration
const (
	jwtSecretKey        = "your-very-secret-key" // CHANGE THIS IN PRODUCTION!
	jwtExpiration       = time.Hour * 24         // Token valid for 24 hours
	refreshTokenNotImpl = ""                     // Placeholder for refresh token
)

// AuthService implements the AuthServiceServer interface defined in proto/api/v1/auth.proto
type AuthService struct {
	pb.UnimplementedAuthServiceServer // Embed for forward compatibility
	userDao                           dao.UserDAO
	// TODO: Add other dependencies like config, token generator, password hasher etc.
}

// NewAuthService creates a new instance of AuthService.
func NewAuthService(userDao dao.UserDAO) *AuthService {
	return &AuthService{
		userDao: userDao,
		// TODO: Initialize other dependencies here
	}
}

// Register handles user registration.
func (s *AuthService) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	// 1. Validate input
	if req.GetUsername() == "" || req.GetEmail() == "" || req.GetPassword() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "username, email, and password are required")
	}
	// TODO: Add more specific validation (e.g., email format, password complexity)

	// 3. Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.GetPassword()), bcrypt.DefaultCost)
	if err != nil {
		// Log the error internally, return a generic error to the client
		// log.Printf("Error hashing password: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to process registration")
	}

	// 4. Store user information in the database.
	newUser := &dao.User{
		Username:     req.GetUsername(),
		Email:        req.GetEmail(),
		PasswordHash: string(hashedPassword),
		// Nickname defaults to username initially or can be empty
		Nickname: req.GetUsername(),
		// CreatedAt/UpdatedAt are set by DAO
	}

	err = s.userDao.CreateUser(ctx, newUser)
	if err != nil {
		if errors.Is(err, dao.ErrUserAlreadyExists) {
			return nil, status.Errorf(codes.AlreadyExists, "username or email already exists")
		}
		// Log the error internally
		// log.Printf("Error creating user: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to register user")
	}

	// 5. Return the registered user information.
	// Note: newUser.ID, CreatedAt, UpdatedAt are populated by CreateUser
	return &pb.RegisterResponse{
		User: mapDaoUserToProto(newUser),
	}, nil
}

// Login handles user login.
func (s *AuthService) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	// 1. Validate input
	loginIdentifier := req.GetLogin()
	password := req.GetPassword()
	if loginIdentifier == "" || password == "" {
		return nil, status.Errorf(codes.InvalidArgument, "login identifier and password are required")
	}

	// 2. Find user by login identifier (username or email).
	user, err := s.userDao.GetUserByLogin(ctx, loginIdentifier)
	if err != nil {
		if errors.Is(err, dao.ErrUserNotFound) {
			return nil, status.Errorf(codes.NotFound, "user not found or invalid credentials")
		}
		// Log internal error
		return nil, status.Errorf(codes.Internal, "failed to process login")
	}

	// 3. Verify password hash.
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		// If err is bcrypt.ErrMismatchedHashAndPassword, it's invalid credentials.
		// Otherwise, it might be an internal error (e.g., invalid hash in DB).
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return nil, status.Errorf(codes.Unauthenticated, "invalid credentials")
		}
		// Log internal error
		return nil, status.Errorf(codes.Internal, "failed to verify password")
	}

	// 4. Generate access token.
	expirationTime := time.Now().Add(jwtExpiration)
	claims := &jwt.RegisteredClaims{
		Subject:   user.ID.Hex(), // Use user's unique ID as subject
		ExpiresAt: jwt.NewNumericDate(expirationTime),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Issuer:    "hyper-sync", // Optional: identify the issuer
		// Add custom claims if needed
		// "username": user.Username,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	accessToken, err := token.SignedString([]byte(jwtSecretKey))
	if err != nil {
		// Log internal error
		return nil, status.Errorf(codes.Internal, "failed to generate access token")
	}

	// TODO: Implement refresh token generation and storage if needed.

	// 5. Return token info and user information.
	tokenInfo := &pb.TokenInfo{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenNotImpl, // Not implemented yet
		ExpiresIn:    int64(jwtExpiration.Seconds()),
	}

	return &pb.LoginResponse{
		TokenInfo: tokenInfo,
		User:      mapDaoUserToProto(user),
	}, nil
}

// getUserIDFromContext simulates retrieving user ID from context (replace with actual implementation).
// This would typically be done by an authentication interceptor.
func getUserIDFromContext(ctx context.Context) (bson.ObjectID, error) {
	userIDStr, ok := ctx.Value(userIDKey).(string)
	if !ok || userIDStr == "" {
		return bson.NilObjectID, status.Errorf(codes.Unauthenticated, "missing user authentication")
	}
	objID, err := bson.ObjectIDFromHex(userIDStr)
	if err != nil {
		return bson.NilObjectID, status.Errorf(codes.Internal, "invalid user ID format in context")
	}
	return objID, nil
}

// UpdateProfile handles updating user profile information.
func (s *AuthService) UpdateProfile(ctx context.Context, req *pb.UpdateProfileRequest) (*pb.UpdateProfileResponse, error) {
	// 1. Get user ID from context (requires authentication middleware).
	userID, err := getUserIDFromContext(ctx) // Replace with actual context retrieval
	if err != nil {
		return nil, err // Return the error from getUserIDFromContext (Unauthenticated or Internal)
	}

	// 2. Validate input (nickname, avatar URL). Must provide at least one.
	updateNickname := req.GetNickname()
	updateAvatar := req.GetAvatarUrl()
	hasUpdate := false
	if req.HasNickname() { // Check if field is explicitly set
		hasUpdate = true
	}
	if req.HasAvatarUrl() { // Check if field is explicitly set
		hasUpdate = true
	}

	if !hasUpdate {
		return nil, status.Errorf(codes.InvalidArgument, "at least one field (nickname or avatar_url) must be provided for update")
	}

	// 3. Fetch current user data
	user, err := s.userDao.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, dao.ErrUserNotFound) {
			// This shouldn't happen if the user ID from context is valid
			return nil, status.Errorf(codes.NotFound, "authenticated user not found")
		}
		// Log internal error
		return nil, status.Errorf(codes.Internal, "failed to retrieve user profile")
	}

	// Update fields if provided in the request
	if req.HasNickname() {
		user.Nickname = updateNickname
	}
	if req.HasAvatarUrl() {
		user.AvatarURL = updateAvatar
	}

	// 4. Update user information in the database.
	err = s.userDao.UpdateUser(ctx, user)
	if err != nil {
		// Log internal error
		return nil, status.Errorf(codes.Internal, "failed to update user profile")
	}

	// 5. Return the updated user information.
	return &pb.UpdateProfileResponse{
		User: mapDaoUserToProto(user),
	}, nil
}

// LoginWithGoogle handles login using a Google account.
func (s *AuthService) LoginWithGoogle(ctx context.Context, req *pb.LoginWithGoogleRequest) (*pb.LoginWithGoogleResponse, error) {
	// TODO: Implement Google login logic:
	// 1. Verify the Google OAuth 2.0 code or ID token.
	// 2. Fetch user info from Google API (email, name, picture).
	// 3. Check if the user exists (by email).
	// 4. If the user doesn't exist, create a new user.
	// 5. Generate access and refresh tokens.
	// 6. Return token info and user information.
	return nil, status.Errorf(codes.Unimplemented, "method LoginWithGoogle not implemented")
}

// GetMe handles retrieving the current user's information.
func (s *AuthService) GetMe(ctx context.Context, req *pb.GetMeRequest) (*pb.GetMeResponse, error) {
	// TODO: Implement get current user logic:
	// 1. Get user ID from context (requires authentication middleware).
	// 2. Query user information from the database.
	// 3. Return user information.
	return nil, status.Errorf(codes.Unimplemented, "method GetMe not implemented")
}

// mapDaoUserToProto converts a DAO User object to a Protobuf User object.
func mapDaoUserToProto(user *dao.User) *pb.User {
	if user == nil {
		return nil
	}
	return &pb.User{
		Id:        user.ID.Hex(), // Convert ObjectID to hex string
		Username:  user.Username,
		Email:     user.Email,
		Nickname:  user.Nickname,
		AvatarUrl: user.AvatarURL,
		CreatedAt: timestamppb.New(user.CreatedAt),
		UpdatedAt: timestamppb.New(user.UpdatedAt),
	}
}
