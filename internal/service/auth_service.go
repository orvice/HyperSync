package service

import (
	"context"
	"errors"
	"fmt" // Add fmt for error formatting
	"log" // Add log for temporary logging
	"os"  // Add os for env vars (temporary)

	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.orx.me/apps/hyper-sync/internal/dao"
	pb "go.orx.me/apps/hyper-sync/pkg/proto/api/v1"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"           // Add oauth2
	"golang.org/x/oauth2/google"    // Add google oauth2 specifics
	"google.golang.org/api/idtoken" // Add idtoken verification
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
	pb.UnimplementedAuthServiceServer
	userDao           dao.UserDAO
	googleOAuthConfig *oauth2.Config // Add Google OAuth config
	// TODO: Replace direct config with a proper config dependency
}

// NewAuthService creates a new instance of AuthService.
// TODO: Inject configuration properly instead of using env vars directly
func NewAuthService(userDao dao.UserDAO) *AuthService {
	// Temporary: Read Google credentials from environment variables
	// In production, use a proper configuration management system (e.g., Viper, envconfig)
	googleClientID := os.Getenv("GOOGLE_CLIENT_ID")
	googleClientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	googleRedirectURL := os.Getenv("GOOGLE_REDIRECT_URL") // URL registered in Google Cloud Console

	if googleClientID == "" || googleClientSecret == "" || googleRedirectURL == "" {
		log.Println("Warning: GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, or GOOGLE_REDIRECT_URL environment variables not set. Google Login will likely fail.")
	}

	oauthCfg := &oauth2.Config{
		ClientID:     googleClientID,
		ClientSecret: googleClientSecret,
		RedirectURL:  googleRedirectURL,                      // Must match one registered in Google Cloud Console
		Scopes:       []string{"openid", "email", "profile"}, // Standard scopes for login
		Endpoint:     google.Endpoint,
	}

	return &AuthService{
		userDao:           userDao,
		googleOAuthConfig: oauthCfg,
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
func getUserIDFromContext(ctx context.Context) (primitive.ObjectID, error) {
	userIDStr, ok := ctx.Value(userIDKey).(string)
	if !ok || userIDStr == "" {
		return primitive.NilObjectID, status.Errorf(codes.Unauthenticated, "missing user authentication")
	}
	objID, err := primitive.ObjectIDFromHex(userIDStr) // Use primitive.ObjectIDFromHex
	if err != nil {
		return primitive.NilObjectID, status.Errorf(codes.Internal, "invalid user ID format in context")
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
	if updateNickname != "" { // Workaround: Check if non-empty instead of HasNickname()
		hasUpdate = true
	}
	if updateAvatar != "" { // Workaround: Check if non-empty instead of HasAvatarUrl()
		hasUpdate = true
	}

	if !hasUpdate {
		return nil, status.Errorf(codes.InvalidArgument, "at least one field (nickname or avatar_url) must be provided for update")
	}

	// 3. Fetch current user data
	user, err := s.userDao.GetUserByID(ctx, userID) // userID is now primitive.ObjectID
	if err != nil {
		if errors.Is(err, dao.ErrUserNotFound) {
			// This shouldn't happen if the user ID from context is valid
			return nil, status.Errorf(codes.NotFound, "authenticated user not found")
		}
		// Log internal error
		return nil, status.Errorf(codes.Internal, "failed to retrieve user profile")
	}

	// Update fields if provided in the request
	if updateNickname != "" { // Workaround: Check if non-empty
		user.Nickname = updateNickname
	}
	if updateAvatar != "" { // Workaround: Check if non-empty
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

// Helper function to generate JWT token (extracted from Login)
func (s *AuthService) generateToken(user *dao.User) (*pb.TokenInfo, error) {
	expirationTime := time.Now().Add(jwtExpiration)
	claims := &jwt.RegisteredClaims{
		Subject:   user.ID.Hex(), // Use user's unique ID as subject
		ExpiresAt: jwt.NewNumericDate(expirationTime),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Issuer:    "hyper-sync", // Optional: identify the issuer
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	accessToken, err := token.SignedString([]byte(jwtSecretKey))
	if err != nil {
		// Log internal error
		log.Printf("Error signing token for user %s: %v", user.ID.Hex(), err)
		return nil, status.Errorf(codes.Internal, "failed to generate access token")
	}

	// TODO: Implement refresh token generation and storage if needed.
	tokenInfo := &pb.TokenInfo{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenNotImpl, // Not implemented yet
		ExpiresIn:    int64(jwtExpiration.Seconds()),
	}
	return tokenInfo, nil
}

// LoginWithGoogle handles login using a Google account.
func (s *AuthService) LoginWithGoogle(ctx context.Context, req *pb.LoginWithGoogleRequest) (*pb.LoginWithGoogleResponse, error) {
	code := req.GetCode()
	if code == "" {
		return nil, status.Errorf(codes.InvalidArgument, "authorization code is required")
	}

	if s.googleOAuthConfig == nil || s.googleOAuthConfig.ClientID == "" {
		log.Println("Error: Google OAuth is not configured.")
		return nil, status.Errorf(codes.Internal, "Google login is not configured on the server")
	}

	// 1. Exchange authorization code for tokens
	// Use NonBlockingContext to avoid depending on the incoming request context deadline
	token, err := s.googleOAuthConfig.Exchange(context.Background(), code)
	if err != nil {
		log.Printf("Error exchanging Google auth code: %v", err)
		return nil, status.Errorf(codes.Unauthenticated, "failed to exchange authorization code with Google: %v", err)
	}

	// Extract the ID token
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		log.Println("Error: Google token response did not contain id_token")
		return nil, status.Errorf(codes.Unauthenticated, "failed to get ID token from Google")
	}

	// 2. Verify the ID token
	// Use context.Background() for the validator as it makes external calls
	idToken, err := idtoken.Validate(context.Background(), rawIDToken, s.googleOAuthConfig.ClientID)
	if err != nil {
		log.Printf("Error validating Google ID token: %v", err)
		return nil, status.Errorf(codes.Unauthenticated, "invalid ID token from Google: %v", err)
	}

	// 3. Extract user info from validated payload
	payload := idToken.Claims
	googleID := payload["sub"].(string) // Subject (Google's unique ID for the user)
	email, _ := payload["email"].(string)
	emailVerified, _ := payload["email_verified"].(bool)
	name, _ := payload["name"].(string)
	// picture, _ := payload["picture"].(string) // Avatar URL

	if email == "" || !emailVerified {
		// Depending on policy, you might reject users without verified emails
		log.Printf("Google login attempt with unverified or missing email for googleID: %s", googleID)
		return nil, status.Errorf(codes.Unauthenticated, "Google account email is missing or not verified")
	}

	// 4. Find or Create User
	var user *dao.User
	user, err = s.userDao.GetUserByGoogleID(ctx, googleID)

	if err != nil && !errors.Is(err, dao.ErrUserNotFound) {
		log.Printf("Error finding user by Google ID %s: %v", googleID, err)
		return nil, status.Errorf(codes.Internal, "failed to query user database")
	}

	if errors.Is(err, dao.ErrUserNotFound) {
		// User not found by Google ID, try by email
		log.Printf("User with Google ID %s not found, trying email %s", googleID, email)
		user, err = s.userDao.GetUserByEmail(ctx, email)

		if err != nil && !errors.Is(err, dao.ErrUserNotFound) {
			log.Printf("Error finding user by email %s: %v", email, err)
			return nil, status.Errorf(codes.Internal, "failed to query user database")
		}

		if errors.Is(err, dao.ErrUserNotFound) {
			// User not found by email either, create a new user
			log.Printf("User with email %s not found, creating new user for Google ID %s", email, googleID)
			newUser := &dao.User{
				// Generate a unique username, perhaps based on email or google id?
				// For now, use email prefix, but this might clash. Needs better strategy.
				Username: fmt.Sprintf("google_%s", googleID), // Placeholder username
				Email:    email,
				Nickname: name, // Use name from Google profile
				// AvatarURL: picture, // Use picture from Google profile
				GoogleID: googleID,
				// PasswordHash is empty as login is via Google
			}
			err = s.userDao.CreateUser(ctx, newUser)
			if err != nil {
				// Handle potential race condition if user was created between checks
				if errors.Is(err, dao.ErrUserAlreadyExists) {
					log.Printf("Race condition? User %s or email %s created concurrently. Retrying lookup.", newUser.Username, email)
					// Retry finding the user, potentially by email again
					user, err = s.userDao.GetUserByEmail(ctx, email)
					if err != nil {
						log.Printf("Error finding user by email %s after create failed: %v", email, err)
						return nil, status.Errorf(codes.Internal, "failed to retrieve user after creation conflict")
					}
				} else {
					log.Printf("Error creating new Google user %s: %v", email, err)
					return nil, status.Errorf(codes.Internal, "failed to create new user")
				}
			} else {
				user = newUser // Use the newly created user
				log.Printf("Successfully created new user for email %s / Google ID %s", email, googleID)
			}
		} else {
			// User found by email, link Google ID
			log.Printf("User found by email %s, linking Google ID %s", email, googleID)
			if user.GoogleID == "" {
				user.GoogleID = googleID
				// Optionally update nickname/avatar if empty or desired
				if user.Nickname == "" && name != "" {
					user.Nickname = name
				}
				// if user.AvatarURL == "" && picture != "" {
				// 	user.AvatarURL = picture
				// }
				err = s.userDao.UpdateUser(ctx, user)
				if err != nil {
					log.Printf("Error linking Google ID %s to user %s: %v", googleID, user.ID.Hex(), err)
					return nil, status.Errorf(codes.Internal, "failed to link Google account")
				}
				log.Printf("Successfully linked Google ID %s to user %s", googleID, user.ID.Hex())
			} else if user.GoogleID != googleID {
				// This email is already linked to a DIFFERENT Google account. This is problematic.
				log.Printf("Error: Email %s is already linked to a different Google ID (%s), cannot link to %s", email, user.GoogleID, googleID)
				return nil, status.Errorf(codes.AlreadyExists, "this email is already associated with a different Google account")
			}
			// If user.GoogleID == googleID, it's already linked, nothing to do.
		}
	}

	// At this point, 'user' variable holds the correct user (found or created)
	if user == nil {
		// Should not happen if logic above is correct
		log.Println("Error: User is nil after find/create logic in LoginWithGoogle")
		return nil, status.Errorf(codes.Internal, "failed to resolve user account")
	}

	// 5. Generate application token
	tokenInfo, err := s.generateToken(user)
	if err != nil {
		// generateToken already logs and returns a gRPC status error
		return nil, err
	}

	// 6. Return response
	return &pb.LoginWithGoogleResponse{
		TokenInfo: tokenInfo,
		User:      mapDaoUserToProto(user),
	}, nil
}

// GetMe handles retrieving the current user's information.
func (s *AuthService) GetMe(ctx context.Context, req *pb.GetMeRequest) (*pb.GetMeResponse, error) {
	// 1. Get user ID from context (requires authentication middleware).
	userID, err := getUserIDFromContext(ctx) // Replace with actual context retrieval if middleware changes
	if err != nil {
		// getUserIDFromContext already returns appropriate gRPC status errors
		return nil, err
	}

	// 2. Query user information from the database.
	user, err := s.userDao.GetUserByID(ctx, userID) // userID is now primitive.ObjectID
	if err != nil {
		if errors.Is(err, dao.ErrUserNotFound) {
			// This indicates an inconsistency, as the user ID from a valid token should exist.
			// Log this potential issue.
			// log.Printf("Error in GetMe: User ID %s from context not found in DB", userID.Hex())
			return nil, status.Errorf(codes.NotFound, "authenticated user not found")
		}
		// Log internal database error
		// log.Printf("Error fetching user %s in GetMe: %v", userID.Hex(), err)
		return nil, status.Errorf(codes.Internal, "failed to retrieve user information")
	}

	// 3. Return user information.
	return &pb.GetMeResponse{
		User: mapDaoUserToProto(user),
	}, nil
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
