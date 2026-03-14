package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"parley/internal/db"
	"parley/internal/email"
)

// User represents an authenticated user
type User struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Email         string `json:"email"`
	AvatarURL     string `json:"avatar_url,omitempty"`
	BannerURL     string `json:"banner_url,omitempty"`
	EmailVerified bool   `json:"email_verified"`
}

// AuthService handles authentication operations
type AuthService struct {
	config      *Config
	repo        *db.Repository
	emailClient *email.Client
	siteURL     string
}

// NewAuthService creates a new AuthService instance
func NewAuthService(repo *db.Repository) *AuthService {
	return &AuthService{
		config:  GetConfig(),
		repo:    repo,
		siteURL: "https://parley.x86-64.com",
	}
}

// SetEmailClient configures the email client and site URL for sending verification emails.
func (s *AuthService) SetEmailClient(client *email.Client, siteURL string) {
	s.emailClient = client
	if siteURL != "" {
		s.siteURL = siteURL
	}
}

// Register creates a new user and returns a token
func (s *AuthService) Register(ctx context.Context, username, email_, password string) (User, string, error) {
	// Validate input
	if username == "" || email_ == "" || password == "" {
		return User{}, "", errors.New("username, email, and password are required")
	}
	if len(username) > 32 {
		return User{}, "", errors.New("username must be 32 characters or fewer")
	}

	// Check if user already exists by email
	_, err := s.repo.GetUserByEmail(ctx, email_)
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

	// Generate email verification token
	verificationToken, err := generateToken()
	if err != nil {
		log.Printf("Register: failed to generate verification token: %v", err)
		verificationToken = "" // fail-open: create user without token
	}

	// Create user in database
	dbUser := &db.User{
		Username:               username,
		Email:                  email_,
		PasswordHash:           hashedPassword,
		EmailVerificationToken: verificationToken,
	}

	err = s.repo.CreateUser(ctx, dbUser)
	if err != nil {
		return User{}, "", err
	}

	// Send verification email (fail-open)
	if s.emailClient != nil && verificationToken != "" {
		if emailErr := s.emailClient.SendVerificationEmail(ctx, email_, username, verificationToken, s.siteURL); emailErr != nil {
			log.Printf("Register: failed to send verification email to %s: %v", email_, emailErr)
		}
	}

	// Convert int64 ID to string for API
	userID := fmt.Sprintf("%d", dbUser.ID)

	user := User{
		ID:            userID,
		Username:      username,
		Email:         email_,
		EmailVerified: false,
	}

	// Generate JWT token
	token, err := s.generateToken(userID)
	if err != nil {
		return User{}, "", err
	}

	return user, token, nil
}

// Login authenticates a user and returns a token
func (s *AuthService) Login(ctx context.Context, email_, password string) (User, string, error) {
	// Validate input
	if email_ == "" || password == "" {
		return User{}, "", errors.New("email and password are required")
	}

	// Look up user by email in database
	dbUser, err := s.repo.GetUserByEmail(ctx, email_)
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
		ID:            userID,
		Username:      dbUser.Username,
		Email:         dbUser.Email,
		AvatarURL:     dbUser.AvatarURL,
		BannerURL:     dbUser.BannerURL,
		EmailVerified: dbUser.EmailVerified,
	}

	// Generate JWT token
	token, err := s.generateToken(userID)
	if err != nil {
		return User{}, "", err
	}

	return user, token, nil
}

// UpdateProfile updates a user's profile fields
func (s *AuthService) UpdateProfile(ctx context.Context, userID, newUsername, currentPassword, newPassword, avatarURL, bannerURL string) (User, error) {
	id, err := fmt.Sscanf(userID, "%d", new(int64))
	_ = id
	if err != nil {
		return User{}, errors.New("invalid user ID")
	}

	var userIDInt int64
	fmt.Sscan(userID, &userIDInt)

	dbUser, err := s.repo.GetUserByID(ctx, userIDInt)
	if err != nil {
		return User{}, errors.New("user not found")
	}

	if newUsername != "" && newUsername != dbUser.Username {
		if len(newUsername) > 32 {
			return User{}, errors.New("username must be 32 characters or fewer")
		}
		// Check username isn't taken
		existing, err := s.repo.GetUserByUsername(ctx, newUsername)
		if err == nil && existing.ID != userIDInt {
			return User{}, errors.New("username already taken")
		}
		dbUser.Username = newUsername
	}

	if newPassword != "" {
		if currentPassword == "" {
			return User{}, errors.New("current password is required to set a new password")
		}
		if !s.CheckPassword(dbUser.PasswordHash, currentPassword) {
			return User{}, errors.New("current password is incorrect")
		}
		hashed := s.HashPassword(newPassword)
		if hashed == "" {
			return User{}, errors.New("failed to hash password")
		}
		dbUser.PasswordHash = hashed
	}

	if avatarURL != "" {
		dbUser.AvatarURL = avatarURL
	}
	if bannerURL != "" {
		dbUser.BannerURL = bannerURL
	}

	if err := s.repo.UpdateUser(ctx, dbUser); err != nil {
		return User{}, err
	}

	return User{
		ID:            userID,
		Username:      dbUser.Username,
		Email:         dbUser.Email,
		AvatarURL:     dbUser.AvatarURL,
		BannerURL:     dbUser.BannerURL,
		EmailVerified: dbUser.EmailVerified,
	}, nil
}

// VerifyEmail marks the user's email as verified using the given token.
func (s *AuthService) VerifyEmail(ctx context.Context, token string) error {
	if token == "" {
		return errors.New("verification token is required")
	}

	dbUser, err := s.repo.GetUserByVerificationToken(ctx, token)
	if err != nil {
		if err == db.ErrNotFound {
			return errors.New("invalid or expired verification token")
		}
		return err
	}

	return s.repo.SetEmailVerified(ctx, dbUser.ID)
}

// ResendVerification sends a new verification email to the user (max 3 resends per day).
func (s *AuthService) ResendVerification(ctx context.Context, userID string) error {
	var userIDInt int64
	if _, err := fmt.Sscan(userID, &userIDInt); err != nil {
		return errors.New("invalid user ID")
	}

	dbUser, err := s.repo.GetUserByID(ctx, userIDInt)
	if err != nil {
		return errors.New("user not found")
	}

	if dbUser.EmailVerified {
		return errors.New("email is already verified")
	}

	if s.emailClient == nil {
		return errors.New("email service is not configured")
	}

	// Check and increment daily resend counter (max 3/day)
	if err := s.repo.CheckAndIncrementEmailResend(ctx, userIDInt); err != nil {
		if err == db.ErrInvalidOperation {
			return errors.New("too many resend attempts today — please try again tomorrow")
		}
		return err
	}

	token, err := generateToken()
	if err != nil {
		return fmt.Errorf("failed to generate verification token: %w", err)
	}

	dbUser.EmailVerificationToken = token
	if err := s.repo.UpdateUser(ctx, dbUser); err != nil {
		return err
	}

	if err := s.emailClient.SendVerificationEmail(ctx, dbUser.Email, dbUser.Username, token, s.siteURL); err != nil {
		return fmt.Errorf("failed to send verification email: %w", err)
	}

	return nil
}

// ChangeEmail changes a user's email address and sends a new verification email.
// Requires password confirmation. Limited to 3 email changes per day.
func (s *AuthService) ChangeEmail(ctx context.Context, userID, newEmail, password string) (User, error) {
	if newEmail == "" {
		return User{}, errors.New("new email is required")
	}
	if password == "" {
		return User{}, errors.New("password is required to change email")
	}

	var userIDInt int64
	if _, err := fmt.Sscan(userID, &userIDInt); err != nil {
		return User{}, errors.New("invalid user ID")
	}

	dbUser, err := s.repo.GetUserByID(ctx, userIDInt)
	if err != nil {
		return User{}, errors.New("user not found")
	}

	if !s.CheckPassword(dbUser.PasswordHash, password) {
		return User{}, errors.New("incorrect password")
	}

	if dbUser.Email == newEmail {
		return User{}, errors.New("new email must be different from current email")
	}

	// Ensure the new email isn't taken by another account
	existing, err := s.repo.GetUserByEmail(ctx, newEmail)
	if err == nil && existing.ID != userIDInt {
		return User{}, errors.New("email address is already in use")
	}
	if err != nil && err != db.ErrNotFound {
		return User{}, err
	}

	// Check and increment daily email-change counter (max 3/day)
	if err := s.repo.CheckAndIncrementEmailChange(ctx, userIDInt); err != nil {
		if err == db.ErrInvalidOperation {
			return User{}, errors.New("too many email changes today — please try again tomorrow")
		}
		return User{}, err
	}

	// Generate verification token for the new address
	token, err := generateToken()
	if err != nil {
		log.Printf("ChangeEmail: failed to generate token: %v", err)
		token = ""
	}

	if err := s.repo.UpdateUserEmail(ctx, userIDInt, newEmail, token); err != nil {
		return User{}, err
	}

	// Send verification email to new address (fail-open)
	if s.emailClient != nil && token != "" {
		if emailErr := s.emailClient.SendVerificationEmail(ctx, newEmail, dbUser.Username, token, s.siteURL); emailErr != nil {
			log.Printf("ChangeEmail: failed to send verification email to %s: %v", newEmail, emailErr)
		}
	}

	return User{
		ID:            userID,
		Username:      dbUser.Username,
		Email:         newEmail,
		AvatarURL:     dbUser.AvatarURL,
		BannerURL:     dbUser.BannerURL,
		EmailVerified: false,
	}, nil
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

// generateToken creates a cryptographically secure random token (64 hex chars).
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
