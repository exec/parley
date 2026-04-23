package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"parley/internal/db"
	"parley/internal/email"
	"parley/internal/validation"
)

// User represents an authenticated user
type User struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Email         string `json:"email"`
	AvatarURL     string `json:"avatar_url,omitempty"`
	BannerURL     string `json:"banner_url,omitempty"`
	Bio           string `json:"bio,omitempty"`
	DisplayName   string `json:"display_name,omitempty"`
	Badges        int    `json:"badges"`
	EmailVerified bool   `json:"email_verified"`
	PhoneNumber   string `json:"phone_number,omitempty"`
	PhoneVerified bool   `json:"phone_verified"`
	HasPassword   bool   `json:"has_password"`
	StatusType    string `json:"status_type,omitempty"`
	StatusText    string `json:"status_text,omitempty"`
}

// AuthService handles authentication operations
type AuthService struct {
	config       *Config
	repo         *db.Repository
	emailClient  *email.Client
	siteURL      string
	sessionCache sync.Map // userID string -> *cachedSessionStatus
}

// sessionCacheTTL governs how long the auth middleware trusts an in-memory
// copy of a user's force-logout / ban state before re-reading the DB. Force
// logouts and bans are rare moderator actions, so a few seconds of staleness
// is acceptable; the upside is that the hot auth path becomes purely local
// memory for the typical request.
const sessionCacheTTL = 3 * time.Second

type cachedSessionStatus struct {
	st        *SessionStatus
	expiresAt time.Time
}

// NewAuthService creates a new AuthService instance
func NewAuthService(repo *db.Repository) *AuthService {
	return &AuthService{
		config: GetConfig(),
		repo:   repo,
	}
}

// InvalidateSessionCache drops the cached session status for userID so the next
// authenticated request goes back to the DB. Call this from the handlers that
// force-logout or ban a user so the change is visible on this node immediately.
// Cross-node invalidation still relies on the TTL.
func (s *AuthService) InvalidateSessionCache(userID string) {
	s.sessionCache.Delete(userID)
}

// PrimeSessionCacheForTests seeds the in-memory session-status cache with the
// given value so GetSessionStatus short-circuits before the DB lookup. It is
// exported solely so handler tests in other packages can drive the session
// path without standing up a database; production callers must never use it.
func (s *AuthService) PrimeSessionCacheForTests(userID string, st *SessionStatus) {
	s.sessionCache.Store(userID, &cachedSessionStatus{st: st, expiresAt: time.Now().Add(time.Hour)})
}

// SetEmailClient configures the email client and site URL for sending verification emails.
func (s *AuthService) SetEmailClient(client *email.Client, siteURL string) {
	s.emailClient = client
	if siteURL != "" {
		s.siteURL = siteURL
	}
}

// Register creates a new user and returns a token. Registration is gated on
// a single-use registration_invites code. The `phone` parameter is accepted
// for API compatibility but ignored — SMS verification was removed while it
// was nonfunctional; reinstating it should restore the commented-out block
// below.
func (s *AuthService) Register(ctx context.Context, username, email_, phone, password, inviteCode, registrationIP string) (User, string, error) {
	username = strings.ToLower(username)

	// Validate input
	if username == "" {
		return User{}, "", errors.New("username is required")
	}
	if inviteCode == "" {
		return User{}, "", errors.New("invite code is required")
	}
	if email_ == "" {
		return User{}, "", errors.New("email is required")
	}
	if len(username) > 32 {
		return User{}, "", errors.New("username must be 32 characters or fewer")
	}
	if !validation.ValidUsername(username) {
		return User{}, "", errors.New("username may only contain letters, numbers, underscores, hyphens and dots")
	}
	if password != "" && len(password) < 8 {
		return User{}, "", errors.New("password must be at least 8 characters")
	}
	if len(password) > 72 {
		return User{}, "", errors.New("password must be 72 characters or fewer (bcrypt limit)")
	}

	// All pre-transaction failure modes below return a single generic error
	// to the caller so registration responses cannot be used as an oracle to
	// enumerate existing usernames, existing emails, or valid invite codes
	// (F-auth-4). Operators still get the precise reason via server logs.
	const genericRegisterErr = "registration failed"

	// Check if user already exists by username
	_, err := s.repo.GetUserByUsername(ctx, username)
	if err == nil {
		log.Printf("Register: duplicate username=%q ip=%s", username, registrationIP)
		return User{}, "", errors.New(genericRegisterErr)
	}
	if err != db.ErrNotFound {
		return User{}, "", err
	}

	// Previous flow accepted phone signups alongside email. SMS verification
	// is disabled for now, so phone-only signups are blocked; the uniqueness
	// check is kept commented for when SMS comes back.
	// if phone != "" {
	// 	_, err = s.repo.GetUserByPhone(ctx, phone)
	// 	if err == nil {
	// 		return User{}, "", errors.New("user with this phone number already exists")
	// 	}
	// 	if err != db.ErrNotFound {
	// 		return User{}, "", err
	// 	}
	// }
	_ = phone // reserved for future SMS reactivation

	if email_ != "" {
		_, err := s.repo.GetUserByEmail(ctx, email_)
		if err == nil {
			log.Printf("Register: duplicate email=%q ip=%s", email_, registrationIP)
			return User{}, "", errors.New(genericRegisterErr)
		}
		if err != db.ErrNotFound {
			return User{}, "", err
		}
	}

	// Hash the password. Empty password means passkey-only account; store an
	// unusable sentinel ("!") that bcrypt will never produce so password login
	// is permanently disabled until the user explicitly sets one.
	var hashedPassword string
	if password == "" {
		hashedPassword = "!"
	} else {
		hashedPassword = s.HashPassword(password)
		if hashedPassword == "" {
			return User{}, "", errors.New("failed to hash password")
		}
	}

	// Generate email verification token. We still store it so the user can
	// trigger verification later via /resend-verification, but we no longer
	// automatically send the email at registration time (see below).
	verificationToken, err := generateToken()
	if err != nil {
		log.Printf("Register: failed to generate verification token: %v", err)
		verificationToken = "" // fail-open: create user without token
	}

	dbUser := &db.User{
		Username:               username,
		Email:                  email_,
		PasswordHash:           hashedPassword,
		EmailVerificationToken: verificationToken,
		// PhoneNumber intentionally left empty while SMS is disabled.
		RegistrationIP: registrationIP,
	}

	// Tie invite consumption + user creation into a single transaction so a
	// failed insert (e.g. username race) doesn't burn the code, and a racing
	// registration against the same code can't produce a zombie user row.
	tx, err := s.repo.DB().BeginTx(ctx, nil)
	if err != nil {
		return User{}, "", err
	}
	defer tx.Rollback()

	var inviterID int64
	err = tx.QueryRowContext(ctx,
		`SELECT inviter_id FROM registration_invites
		 WHERE code = $1 AND used_at IS NULL
		 FOR UPDATE`, inviteCode,
	).Scan(&inviterID)
	if err == sql.ErrNoRows {
		log.Printf("Register: invalid or used invite code=%q ip=%s", inviteCode, registrationIP)
		return User{}, "", errors.New(genericRegisterErr)
	}
	if err != nil {
		return User{}, "", err
	}

	if err := s.repo.CreateUserTx(ctx, tx, dbUser); err != nil {
		return User{}, "", err
	}

	if err := s.repo.ConsumeRegistrationInvite(ctx, tx, inviteCode, dbUser.ID); err != nil {
		return User{}, "", err
	}

	if err := tx.Commit(); err != nil {
		return User{}, "", err
	}

	// Create default theme preferences (fail-open)
	if prefErr := s.repo.CreateUserPreferences(ctx, dbUser.ID); prefErr != nil {
		log.Printf("Register: failed to create user_preferences for user %d: %v", dbUser.ID, prefErr)
	}

	// Invite-only launch: don't auto-send the verification email. The user
	// can request one via POST /api/auth/resend-verification when ready.
	// if s.emailClient != nil && email_ != "" && verificationToken != "" {
	// 	if emailErr := s.emailClient.SendVerificationEmail(ctx, email_, username, verificationToken, s.siteURL); emailErr != nil {
	// 		log.Printf("Register: failed to send verification email to %s: %v", email_, emailErr)
	// 	}
	// }

	// SMS verification send removed while the SMS provider is nonfunctional.
	// if s.emailClient != nil && phone != "" {
	// 	smsCode, codeErr := generateSMSCode()
	// 	if codeErr == nil {
	// 		expiresAt := time.Now().Add(15 * time.Minute)
	// 		if dbErr := s.repo.SetPhoneVerificationCode(ctx, dbUser.ID, smsCode, expiresAt); dbErr == nil {
	// 			if smsErr := s.emailClient.SendVerificationSMS(ctx, phone, smsCode); smsErr != nil {
	// 				log.Printf("Register: failed to send SMS to %s: %v", phone, smsErr)
	// 			}
	// 		}
	// 	}
	// }

	// Convert int64 ID to string for API
	userID := fmt.Sprintf("%d", dbUser.ID)

	user := User{
		ID:            userID,
		Username:      username,
		Email:         email_,
		EmailVerified: false,
		PhoneVerified: false,
		HasPassword:   hashedPassword != "!",
	}

	// Generate JWT token
	token, err := s.generateToken(userID)
	if err != nil {
		return User{}, "", err
	}

	return user, token, nil
}

// dbUserToUser converts a db.User to an auth.User. The ID must be pre-formatted.
func dbUserToUser(dbUser *db.User, id string) User {
	return User{
		ID:            id,
		Username:      dbUser.Username,
		Email:         dbUser.Email,
		AvatarURL:     dbUser.AvatarURL,
		BannerURL:     dbUser.BannerURL,
		Bio:           dbUser.Bio,
		DisplayName:   dbUser.DisplayName,
		Badges:        dbUser.Badges,
		EmailVerified: dbUser.EmailVerified,
		PhoneNumber:   dbUser.PhoneNumber,
		PhoneVerified: dbUser.PhoneVerified,
		HasPassword:   dbUser.PasswordHash != "!",
		StatusType:    dbUser.StatusType,
		StatusText:    dbUser.StatusText,
	}
}

// RemovePassword sets the user's password to the unusable sentinel "!".
// Requires at least one passkey to be registered (enforced by the caller).
func (s *AuthService) RemovePassword(ctx context.Context, userIDStr string) error {
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return errors.New("invalid user ID")
	}
	return s.repo.UpdatePasswordHash(ctx, userID, "!")
}

// UpdateStatus updates a user's status type and status text.
func (s *AuthService) UpdateStatus(ctx context.Context, userIDStr, statusType, statusText string) error {
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return errors.New("invalid user ID")
	}
	// Validate status_type
	switch statusType {
	case "online", "dnd", "afk", "invisible":
		// valid
	default:
		return errors.New("invalid status type")
	}
	return s.repo.UpdateUserStatus(ctx, userID, statusType, statusText)
}

// Login authenticates a user and returns a token
func (s *AuthService) Login(ctx context.Context, emailOrPhone, password, ip string) (User, string, error) {
	if emailOrPhone == "" || password == "" {
		return User{}, "", errors.New("email/phone and password are required")
	}

	var dbUser *db.User
	var err error

	// Try email first, then phone
	dbUser, err = s.repo.GetUserByEmail(ctx, emailOrPhone)
	if err == db.ErrNotFound {
		dbUser, err = s.repo.GetUserByPhone(ctx, emailOrPhone)
	}
	if err != nil {
		if err == db.ErrNotFound {
			return User{}, "", errors.New("invalid credentials")
		}
		return User{}, "", err
	}

	if dbUser.BannedAt != nil {
		reason := "violation of Terms of Service"
		if dbUser.BanReason != "" {
			reason = dbUser.BanReason
		}
		log.Printf("audit: login_blocked_banned user_id=%d ip=%s reason=%q", dbUser.ID, ip, reason)
		return User{}, "", fmt.Errorf("Your account was dissolved in a vat of acid. Reason: %s. Appeals can be submitted to /dev/null.", reason)
	}

	if !s.CheckPassword(dbUser.PasswordHash, password) {
		return User{}, "", errors.New("invalid credentials")
	}

	if ip != "" {
		_ = s.repo.UpdateLastSeenIP(ctx, dbUser.ID, ip)
	}

	userID := fmt.Sprintf("%d", dbUser.ID)
	user := dbUserToUser(dbUser, userID)

	token, err := s.generateToken(userID)
	if err != nil {
		return User{}, "", err
	}
	log.Printf("audit: login_success user_id=%d ip=%s", dbUser.ID, ip)
	return user, token, nil
}

// UpdateProfile updates a user's profile fields
func (s *AuthService) UpdateProfile(ctx context.Context, userID, newUsername, currentPassword, newPassword, avatarURL, bannerURL string, bio, displayName *string) (User, error) {
	userIDInt, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return User{}, errors.New("invalid user ID")
	}

	dbUser, err := s.repo.GetUserByID(ctx, userIDInt)
	if err != nil {
		return User{}, errors.New("user not found")
	}

	if newUsername != "" {
		newUsername = strings.ToLower(newUsername)
	}

	if newUsername != "" && newUsername != dbUser.Username {
		if len(newUsername) > 32 {
			return User{}, errors.New("username must be 32 characters or fewer")
		}
		if !validation.ValidUsername(newUsername) {
			return User{}, errors.New("username may only contain letters, numbers, underscores, hyphens and dots")
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
		if len(newPassword) < 8 {
			return User{}, errors.New("password must be at least 8 characters")
		}
		if len(newPassword) > 72 {
			return User{}, errors.New("password must be 72 characters or fewer (bcrypt limit)")
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
	if bio != nil {
		if validation.HasSpoofedLink(*bio) {
			return User{}, errors.New("bio contains a spoofed link")
		}
		dbUser.Bio = *bio
	}
	if displayName != nil {
		if len(*displayName) > 32 {
			return User{}, errors.New("display name must be 32 characters or fewer")
		}
		dbUser.DisplayName = *displayName
	}

	if err := s.repo.UpdateUser(ctx, dbUser); err != nil {
		return User{}, err
	}

	return dbUserToUser(dbUser, userID), nil
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

	u := dbUserToUser(dbUser, userID)
	u.Email = newEmail
	u.EmailVerified = false
	return u, nil
}

// RequestPasswordReset sends a password reset email to the given address.
// Always returns nil to prevent user enumeration.
func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) error {
	if email == "" {
		return nil
	}
	dbUser, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return nil // user not found — don't reveal this
	}
	if s.emailClient == nil {
		return nil
	}
	token, err := generateToken()
	if err != nil {
		log.Printf("RequestPasswordReset: failed to generate token: %v", err)
		return nil
	}
	expiresAt := time.Now().Add(1 * time.Hour)
	if err := s.repo.SetPasswordResetToken(ctx, dbUser.ID, token, expiresAt); err != nil {
		log.Printf("RequestPasswordReset: failed to set token: %v", err)
		return nil
	}
	if err := s.emailClient.SendPasswordResetEmail(ctx, dbUser.Email, dbUser.Username, token, s.siteURL); err != nil {
		log.Printf("RequestPasswordReset: failed to send email to %s: %v", dbUser.Email, err)
	}
	return nil
}

// ResetPassword sets a new password using a valid reset token.
func (s *AuthService) ResetPassword(ctx context.Context, token, newPassword string) error {
	if token == "" {
		return errors.New("reset token is required")
	}
	if len(newPassword) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if len(newPassword) > 72 {
		return errors.New("password must be 72 characters or fewer (bcrypt limit)")
	}
	hashed := s.HashPassword(newPassword)
	if hashed == "" {
		return errors.New("failed to hash password")
	}
	if err := s.repo.ConsumePasswordResetToken(ctx, token, hashed); err != nil {
		if err == db.ErrInvalidOperation {
			return errors.New("invalid or expired reset token")
		}
		return err
	}
	return nil
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

// TokenInfo surfaces the claims that callers need from a validated user JWT.
// IsImpersonation + ActorAdminID flow through the middleware so downstream
// handlers and audit logs can tell support-session traffic from a real user
// session (see audit finding F-impersonation-claim).
type TokenInfo struct {
	UserID          string
	IssuedAt        int64
	ExpiresAt       int64
	IsImpersonation bool
	// ActorAdminID is the admin_users.id of the admin who minted the
	// impersonation token. Empty when IsImpersonation is false.
	ActorAdminID string
}

// ValidateTokenFull parses and validates a user JWT and returns the full claim
// set the auth layer cares about. Callers that only need the userID should use
// ValidateToken; callers that inspect iat (force-logout check) or the
// impersonation flag (audit log / denyImpersonation middleware) use this one.
//
// Key selection (F-admin-jwt-secret):
//
//   - Normal user tokens are signed with SecretKey (JWT_SECRET). They must not
//     carry an `impersonation: true` claim.
//   - Impersonation tokens are signed with ImpersonationSecretKey
//     (IMPERSONATION_JWT_SECRET) and must carry `impersonation: true`.
//
// The validator peeks at the `impersonation` claim first (unverified), then
// picks the signing key to check against. This enforces the invariant that a
// compromise of one secret cannot produce the other kind of token — even if
// an attacker with IMPERSONATION_JWT_SECRET strips the claim, the signature
// will fail against SecretKey. Never try both keys blindly: a claim-shape
// attack could slip past if you did.
func (s *AuthService) ValidateTokenFull(tokenString string) (TokenInfo, error) {
	if tokenString == "" {
		return TokenInfo{}, errors.New("token is required")
	}

	// First pass: parse without verifying the signature so we can read the
	// `impersonation` claim and pick the right key.
	unverified, _, err := jwt.NewParser().ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return TokenInfo{}, err
	}
	if _, ok := unverified.Method.(*jwt.SigningMethodHMAC); !ok {
		return TokenInfo{}, errors.New("invalid signing method")
	}
	unverifiedClaims, ok := unverified.Claims.(jwt.MapClaims)
	if !ok {
		return TokenInfo{}, errors.New("invalid token")
	}
	impClaim, _ := unverifiedClaims["impersonation"].(bool)

	// Select the signing key based on the (unverified) impersonation claim.
	// Mismatched signatures get rejected in the verify pass below.
	var key []byte
	if impClaim {
		if s.config.ImpersonationSecretKey == "" {
			return TokenInfo{}, errors.New("impersonation unavailable")
		}
		key = []byte(s.config.ImpersonationSecretKey)
	} else {
		key = []byte(s.config.SecretKey)
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("invalid signing method")
		}
		return key, nil
	})
	if err != nil {
		if impClaim {
			return TokenInfo{}, errors.New("invalid impersonation token")
		}
		return TokenInfo{}, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return TokenInfo{}, errors.New("invalid token")
	}
	expF, ok := claims["exp"].(float64)
	if !ok {
		return TokenInfo{}, errors.New("invalid token claims")
	}
	exp := int64(expF)
	if time.Now().Unix() > exp {
		return TokenInfo{}, errors.New("token has expired")
	}
	uid, ok := claims["user_id"].(string)
	if !ok {
		return TokenInfo{}, errors.New("invalid user_id in token")
	}
	info := TokenInfo{UserID: uid, ExpiresAt: exp}
	if iatF, ok := claims["iat"].(float64); ok {
		info.IssuedAt = int64(iatF)
	}
	if imp, ok := claims["impersonation"].(bool); ok && imp {
		info.IsImpersonation = true
		// actor_admin_id is set by admin's handleImpersonate; tokens
		// from older code paths may lack it — treat as unknown actor.
		if actor, ok := claims["actor_admin_id"].(string); ok {
			info.ActorAdminID = actor
		}
	}
	return info, nil
}

// ValidateToken validates a JWT token and returns the userID. Back-compat
// wrapper over ValidateTokenFull for callers that don't need the richer
// claim set.
func (s *AuthService) ValidateToken(tokenString string) (string, error) {
	info, err := s.ValidateTokenFull(tokenString)
	if err != nil {
		return "", err
	}
	return info.UserID, nil
}

// SessionStatus carries the auth-relevant row fields the middleware checks on
// every request: force-logout timestamp and ban state. Merging both into a
// single lookup avoids two round-trips-per-request through pgbouncer.
type SessionStatus struct {
	ForceLogoutAt sql.NullTime
	BannedAt      sql.NullTime
	BanReason     sql.NullString
}

// GetSessionStatus fetches force-logout + ban state in a single query.
// Middleware should prefer this over calling IsForceLoggedOut and IsBanned
// separately; the individual methods are kept for callers that only need one.
//
// Results are cached in-process for sessionCacheTTL — force-logout and ban
// are rare moderator actions, so a few seconds of staleness is acceptable
// and the cache turns the hot path into a local-memory read.
func (s *AuthService) GetSessionStatus(ctx context.Context, userID string) (*SessionStatus, error) {
	if v, ok := s.sessionCache.Load(userID); ok {
		e := v.(*cachedSessionStatus)
		if time.Now().Before(e.expiresAt) {
			return e.st, nil
		}
	}
	id, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid user ID")
	}
	var st SessionStatus
	err = s.repo.DB().QueryRowContext(ctx,
		`SELECT force_logout_at, banned_at, ban_reason FROM users WHERE id = $1`, id,
	).Scan(&st.ForceLogoutAt, &st.BannedAt, &st.BanReason)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	s.sessionCache.Store(userID, &cachedSessionStatus{st: &st, expiresAt: time.Now().Add(sessionCacheTTL)})
	return &st, nil
}

// IsBanned returns whether a user is currently banned and (if so) the reason.
// Mirrors the shape of IsForceLoggedOut; called from auth middleware on every
// authenticated request so a ban takes effect immediately even for users with
// a still-valid JWT.
func (s *AuthService) IsBanned(ctx context.Context, userID string) (bool, string, error) {
	id, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return false, "", errors.New("invalid user ID")
	}
	var bannedAt sql.NullTime
	var banReason sql.NullString
	err = s.repo.DB().QueryRowContext(ctx,
		`SELECT banned_at, ban_reason FROM users WHERE id = $1`, id,
	).Scan(&bannedAt, &banReason)
	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	if bannedAt.Valid {
		return true, banReason.String, nil
	}
	return false, "", nil
}

// IsForceLoggedOut checks if a token issued at issuedAt should be invalidated due to a force logout.
func (s *AuthService) IsForceLoggedOut(ctx context.Context, userID string, issuedAt int64) (bool, error) {
	id, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return false, errors.New("invalid user ID")
	}
	var forceLogoutAt sql.NullTime
	err = s.repo.DB().QueryRowContext(ctx,
		`SELECT force_logout_at FROM users WHERE id = $1`, id,
	).Scan(&forceLogoutAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if forceLogoutAt.Valid && issuedAt <= forceLogoutAt.Time.Unix() {
		return true, nil
	}
	return false, nil
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

// GetUserByID loads a user by string ID and returns the API User struct.
func (s *AuthService) GetUserByID(ctx context.Context, userIDStr string) (User, error) {
	var id int64
	if _, err := fmt.Sscan(userIDStr, &id); err != nil {
		return User{}, errors.New("invalid user ID")
	}
	dbUser, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		return User{}, errors.New("user not found")
	}
	return dbUserToUser(dbUser, userIDStr), nil
}

// GenerateTokenForUser creates a JWT for a given user ID string.
func (s *AuthService) GenerateTokenForUser(userIDStr string) (string, error) {
	return s.generateToken(userIDStr)
}

// generateToken creates a cryptographically secure random token (64 hex chars).
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// generateSMSCode creates a cryptographically random 6-digit numeric OTP.
func generateSMSCode() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	n := (int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])) & 0x7fffffff
	return fmt.Sprintf("%06d", n%1000000), nil
}

// SendPhoneVerification generates and sends a new 6-digit OTP to the user's phone.
func (s *AuthService) SendPhoneVerification(ctx context.Context, userID string) error {
	var id int64
	if _, err := fmt.Sscan(userID, &id); err != nil {
		return errors.New("invalid user ID")
	}
	dbUser, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		return errors.New("user not found")
	}
	if dbUser.PhoneNumber == "" {
		return errors.New("no phone number on account")
	}
	if dbUser.PhoneVerified {
		return errors.New("phone is already verified")
	}
	if s.emailClient == nil {
		return errors.New("SMS service is not configured")
	}
	if err := s.repo.CheckAndIncrementSmsResend(ctx, id); err != nil {
		if err == db.ErrInvalidOperation {
			return errors.New("too many SMS attempts today — please try again tomorrow")
		}
		return err
	}
	code, err := generateSMSCode()
	if err != nil {
		return fmt.Errorf("failed to generate code: %w", err)
	}
	expiresAt := time.Now().Add(15 * time.Minute)
	if err := s.repo.SetPhoneVerificationCode(ctx, id, code, expiresAt); err != nil {
		return err
	}
	return s.emailClient.SendVerificationSMS(ctx, dbUser.PhoneNumber, code)
}

// VerifyPhone confirms a user's phone number using the OTP they received.
func (s *AuthService) VerifyPhone(ctx context.Context, userID, code string) error {
	var id int64
	if _, err := fmt.Sscan(userID, &id); err != nil {
		return errors.New("invalid user ID")
	}
	if err := s.repo.CheckPhoneVerificationCode(ctx, id, code); err != nil {
		if err == db.ErrInvalidOperation {
			return errors.New("invalid or expired verification code")
		}
		return err
	}
	return s.repo.SetPhoneVerified(ctx, id)
}

// ChangePhone updates a user's phone number and sends a new OTP.
func (s *AuthService) ChangePhone(ctx context.Context, userID, newPhone, password string) (User, error) {
	if newPhone == "" {
		return User{}, errors.New("phone number is required")
	}
	if password == "" {
		return User{}, errors.New("password is required to change phone")
	}
	var id int64
	if _, err := fmt.Sscan(userID, &id); err != nil {
		return User{}, errors.New("invalid user ID")
	}
	dbUser, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		return User{}, errors.New("user not found")
	}
	if !s.CheckPassword(dbUser.PasswordHash, password) {
		return User{}, errors.New("incorrect password")
	}
	if dbUser.PhoneNumber == newPhone {
		return User{}, errors.New("new phone must be different from current phone")
	}
	existing, err := s.repo.GetUserByPhone(ctx, newPhone)
	if err == nil && existing.ID != id {
		return User{}, errors.New("phone number is already in use")
	}
	if err != nil && err != db.ErrNotFound {
		return User{}, err
	}
	if err := s.repo.UpdateUserPhone(ctx, id, newPhone); err != nil {
		return User{}, err
	}
	// Send verification SMS (fail-open)
	if s.emailClient != nil {
		code, codeErr := generateSMSCode()
		if codeErr == nil {
			expiresAt := time.Now().Add(15 * time.Minute)
			if setErr := s.repo.SetPhoneVerificationCode(ctx, id, code, expiresAt); setErr == nil {
				if smsErr := s.emailClient.SendVerificationSMS(ctx, newPhone, code); smsErr != nil {
					log.Printf("ChangePhone: failed to send SMS: %v", smsErr)
				}
			}
		}
	}
	u := dbUserToUser(dbUser, userID)
	u.PhoneNumber = newPhone
	u.PhoneVerified = false
	return u, nil
}
