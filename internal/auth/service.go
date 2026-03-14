package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"parley/internal/db"
)

// User represents an authenticated user
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

// AuthService handles authentication operations
type AuthService struct {
	config *Config
	repo   *db.Repository
}

// NewAuthService creates a new AuthService instance
func NewAuthService(repo *db.Repository) *AuthService {
	return &AuthService{
		config: GetConfig(),
		repo:   repo,
	}
}

// Register creates a new user and returns a token
func (s *AuthService) Register(ctx context.Context, username, email, password string) (User, string, error) {
	// Validate input
	if username == "" || email == "" || password == "" {
		return User{}, "", errors.New("username, email, and password are required")
	}

	// Check if user already exists by email
	_, err := s.repo.GetUserByEmail(ctx, email)
	if err == nil {
		return User{}, "", errors.New("user with this email already exists")
	}
	if err != db.ErrNotFound {
		return User{}, "", err
	}

	// Check if user already exists by username
	_, err = s.repo.GetUserByUsername(ctx, username)
	if err == nil {
		return User{}, "", errors.New("user with this username already exists")
	}
	if err != db.ErrNotFound {
		return User{}, "", err
	}

	// Hash the password
	hashedPassword := s.HashPassword(password)
	if hashedPassword == "" {
		return User{}, "", errors.New("failed to hash password")
	}

	// Create user in database
	dbUser := &db.User{
		Username:     username,
		Email:        email,
		PasswordHash: hashedPassword,
	}

	err = s.repo.CreateUser(ctx, dbUser)
	if err != nil {
		return User{}, "", err
	}

	// Convert int64 ID to string for API
	userID := fmt.Sprintf("%d", dbUser.ID)

	user := User{
		ID:       userID,
		Username: username,
		Email:    email,
	}

	// Generate JWT token
	token, err := s.generateToken(userID)
	if err != nil {
		return User{}, "", err
	}

	return user, token, nil
}

// Login authenticates a user and returns a token
func (s *AuthService) Login(ctx context.Context, email, password string) (User, string, error) {
	// Validate input
	if email == "" || password == "" {
		return User{}, "", errors.New("email and password are required")
	}

	// Look up user by email in database
	dbUser, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if err == db.ErrNotFound {
			return User{}, "", errors.New("invalid credentials")
		}
		return User{}, "", err
	}

	// Verify password
	if !s.CheckPassword(dbUser.PasswordHash, password) {
		return User{}, "", errors.New("invalid credentials")
	}

	// Convert int64 ID to string for API
	userID := fmt.Sprintf("%d", dbUser.ID)

	user := User{
		ID:       userID,
		Username: dbUser.Username,
		Email:    dbUser.Email,
	}

	// Generate JWT token
	token, err := s.generateToken(userID)
	if err != nil {
		return User{}, "", err
	}

	return user, token, nil
}

// HashPassword creates a bcrypt hash of the password
func (s *AuthService) HashPassword(password string) string {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return ""
	}
	return string(bytes)
}

// CheckPassword verifies a password against a hash
func (s *AuthService) CheckPassword(hash, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// ValidateToken validates a JWT token and returns the userID
func (s *AuthService) ValidateToken(tokenString string) (string, error) {
	if tokenString == "" {
		return "", errors.New("token is required")
	}

	// Parse the token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Validate the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("invalid signing method")
		}
		return []byte(s.config.SecretKey), nil
	})

	if err != nil {
		return "", err
	}

	// Extract claims
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		// Check expiration
		exp, ok := claims["exp"].(float64)
		if !ok {
			return "", errors.New("invalid token claims")
		}

		if time.Now().Unix() > int64(exp) {
			return "", errors.New("token has expired")
		}

		// Extract userID
		userID, ok := claims["user_id"].(string)
		if !ok {
			return "", errors.New("invalid user_id in token")
		}

		return userID, nil
	}

	return "", errors.New("invalid token")
}

// generateToken creates a new JWT token for a user
func (s *AuthService) generateToken(userID string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(s.config.TokenExpiry).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.config.SecretKey))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}