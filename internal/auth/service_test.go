package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// newTestService creates an AuthService with a known secret for unit tests.
// It bypasses the DB-dependent config init by setting the config directly.
func newTestService() *AuthService {
	return &AuthService{
		config: &Config{
			SecretKey:   "test-secret-key-for-unit-tests",
			TokenExpiry: 24 * time.Hour,
		},
	}
}

// --- HashPassword / CheckPassword ---

func TestHashPasswordAndCheck(t *testing.T) {
	svc := newTestService()
	hash := svc.HashPassword("correcthorsebatterystaple")
	if hash == "" {
		t.Fatal("HashPassword returned empty string")
	}
	if !svc.CheckPassword(hash, "correcthorsebatterystaple") {
		t.Error("CheckPassword should return true for the correct password")
	}
}

func TestCheckPasswordWrongPassword(t *testing.T) {
	svc := newTestService()
	hash := svc.HashPassword("rightpassword")
	if svc.CheckPassword(hash, "wrongpassword") {
		t.Error("CheckPassword should return false for the wrong password")
	}
}

func TestCheckPasswordUnusableSentinel(t *testing.T) {
	svc := newTestService()
	// "!" is the sentinel for passkey-only accounts — should never match any password
	if svc.CheckPassword("!", "anything") {
		t.Error("CheckPassword should return false for the unusable sentinel '!'")
	}
}

func TestCheckPasswordEmptyHash(t *testing.T) {
	svc := newTestService()
	if svc.CheckPassword("", "password") {
		t.Error("CheckPassword should return false for empty hash")
	}
}

// --- generateToken (JWT) ---

func TestGenerateTokenAndValidate(t *testing.T) {
	svc := newTestService()
	token, err := svc.generateToken("42")
	if err != nil {
		t.Fatalf("generateToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("generateToken returned empty string")
	}

	userID, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if userID != "42" {
		t.Errorf("expected userID '42', got '%s'", userID)
	}
}

func TestValidateTokenEmpty(t *testing.T) {
	svc := newTestService()
	_, err := svc.ValidateToken("")
	if err == nil {
		t.Error("ValidateToken should reject empty token")
	}
}

func TestValidateTokenExpired(t *testing.T) {
	svc := newTestService()
	// Create a token that expired 1 hour ago
	claims := jwt.MapClaims{
		"user_id": "42",
		"exp":     time.Now().Add(-1 * time.Hour).Unix(),
		"iat":     time.Now().Add(-2 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(svc.config.SecretKey))
	if err != nil {
		t.Fatalf("failed to create test token: %v", err)
	}

	_, err = svc.ValidateToken(tokenString)
	if err == nil {
		t.Error("ValidateToken should reject expired token")
	}
}

func TestValidateTokenWrongSecret(t *testing.T) {
	svc := newTestService()
	// Sign with a different key
	claims := jwt.MapClaims{
		"user_id": "42",
		"exp":     time.Now().Add(1 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte("wrong-secret"))
	if err != nil {
		t.Fatalf("failed to create test token: %v", err)
	}

	_, err = svc.ValidateToken(tokenString)
	if err == nil {
		t.Error("ValidateToken should reject token signed with wrong secret")
	}
}

func TestValidateTokenWrongSigningMethod(t *testing.T) {
	svc := newTestService()
	// Use RSA "none" attack: create a token with "none" alg
	claims := jwt.MapClaims{
		"user_id": "42",
		"exp":     time.Now().Add(1 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("failed to create test token: %v", err)
	}

	_, err = svc.ValidateToken(tokenString)
	if err == nil {
		t.Error("ValidateToken should reject token with 'none' signing method")
	}
}

func TestValidateTokenMissingUserID(t *testing.T) {
	svc := newTestService()
	claims := jwt.MapClaims{
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(svc.config.SecretKey))
	if err != nil {
		t.Fatalf("failed to create test token: %v", err)
	}

	_, err = svc.ValidateToken(tokenString)
	if err == nil {
		t.Error("ValidateToken should reject token missing user_id claim")
	}
}

func TestValidateTokenMissingExp(t *testing.T) {
	svc := newTestService()
	claims := jwt.MapClaims{
		"user_id": "42",
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(svc.config.SecretKey))
	if err != nil {
		t.Fatalf("failed to create test token: %v", err)
	}

	_, err = svc.ValidateToken(tokenString)
	if err == nil {
		t.Error("ValidateToken should reject token missing exp claim")
	}
}

// --- ValidateTokenFull ---

func TestValidateTokenFullReturnsIAT(t *testing.T) {
	svc := newTestService()
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id": "99",
		"exp":     now.Add(1 * time.Hour).Unix(),
		"iat":     now.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(svc.config.SecretKey))
	if err != nil {
		t.Fatalf("failed to create test token: %v", err)
	}

	userID, iat, err := svc.ValidateTokenFull(tokenString)
	if err != nil {
		t.Fatalf("ValidateTokenFull failed: %v", err)
	}
	if userID != "99" {
		t.Errorf("expected userID '99', got '%s'", userID)
	}
	if iat != now.Unix() {
		t.Errorf("expected iat %d, got %d", now.Unix(), iat)
	}
}

func TestValidateTokenFullEmpty(t *testing.T) {
	svc := newTestService()
	_, _, err := svc.ValidateTokenFull("")
	if err == nil {
		t.Error("ValidateTokenFull should reject empty token")
	}
}

// --- GenerateImpersonationToken ---

func TestGenerateImpersonationToken(t *testing.T) {
	svc := newTestService()
	token, err := svc.GenerateImpersonationToken("42")
	if err != nil {
		t.Fatalf("GenerateImpersonationToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("GenerateImpersonationToken returned empty string")
	}

	// Parse and check the impersonation claim
	parsed, err := jwt.Parse(token, func(tok *jwt.Token) (interface{}, error) {
		return []byte(svc.config.SecretKey), nil
	})
	if err != nil {
		t.Fatalf("failed to parse impersonation token: %v", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("unexpected claims type")
	}
	if claims["user_id"] != "42" {
		t.Errorf("expected user_id '42', got '%v'", claims["user_id"])
	}
	if claims["impersonation"] != true {
		t.Errorf("expected impersonation=true, got '%v'", claims["impersonation"])
	}
}

func TestGenerateImpersonationTokenEmptyID(t *testing.T) {
	svc := newTestService()
	_, err := svc.GenerateImpersonationToken("")
	if err == nil {
		t.Error("GenerateImpersonationToken should reject empty user ID")
	}
}

// --- GenerateTokenForUser ---

func TestGenerateTokenForUser(t *testing.T) {
	svc := newTestService()
	token, err := svc.GenerateTokenForUser("77")
	if err != nil {
		t.Fatalf("GenerateTokenForUser failed: %v", err)
	}

	userID, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if userID != "77" {
		t.Errorf("expected userID '77', got '%s'", userID)
	}
}

// --- Token expiry duration ---

func TestTokenExpiryDuration(t *testing.T) {
	svc := &AuthService{
		config: &Config{
			SecretKey:   "test-secret",
			TokenExpiry: 5 * time.Minute,
		},
	}
	token, err := svc.generateToken("1")
	if err != nil {
		t.Fatalf("generateToken failed: %v", err)
	}

	parsed, err := jwt.Parse(token, func(tok *jwt.Token) (interface{}, error) {
		return []byte("test-secret"), nil
	})
	if err != nil {
		t.Fatalf("failed to parse token: %v", err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	exp := int64(claims["exp"].(float64))
	iat := int64(claims["iat"].(float64))
	diff := exp - iat
	// Allow 2 second tolerance for test execution time
	if diff < 298 || diff > 302 {
		t.Errorf("expected ~300s token lifetime, got %ds", diff)
	}
}

// --- generateToken (random hex) ---

func TestGenerateRandomToken(t *testing.T) {
	tok1, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken failed: %v", err)
	}
	if len(tok1) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(tok1))
	}

	tok2, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken failed: %v", err)
	}
	if tok1 == tok2 {
		t.Error("two sequential tokens should not be identical")
	}
}

// --- generateSMSCode ---

func TestGenerateSMSCode(t *testing.T) {
	code, err := generateSMSCode()
	if err != nil {
		t.Fatalf("generateSMSCode failed: %v", err)
	}
	if len(code) != 6 {
		t.Errorf("expected 6-digit code, got length %d: %s", len(code), code)
	}
	for _, ch := range code {
		if ch < '0' || ch > '9' {
			t.Errorf("SMS code contains non-digit character: %c", ch)
		}
	}
}

func TestGenerateSMSCodeUniqueness(t *testing.T) {
	// Generate 100 codes and check they aren't all identical
	codes := make(map[string]bool)
	for i := 0; i < 100; i++ {
		code, err := generateSMSCode()
		if err != nil {
			t.Fatalf("generateSMSCode failed on iteration %d: %v", i, err)
		}
		codes[code] = true
	}
	if len(codes) < 10 {
		t.Errorf("expected diversity in 100 SMS codes, got only %d unique values", len(codes))
	}
}

// --- SHA256Hex ---

func TestSHA256Hex(t *testing.T) {
	// Known SHA-256 of "hello"
	expected := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	got := SHA256Hex("hello")
	if got != expected {
		t.Errorf("SHA256Hex(\"hello\") = %s, want %s", got, expected)
	}
}

func TestSHA256HexDeterministic(t *testing.T) {
	a := SHA256Hex("test-input")
	b := SHA256Hex("test-input")
	if a != b {
		t.Error("SHA256Hex should be deterministic")
	}
}

func TestSHA256HexDifferentInputs(t *testing.T) {
	a := SHA256Hex("input-a")
	b := SHA256Hex("input-b")
	if a == b {
		t.Error("SHA256Hex should produce different outputs for different inputs")
	}
}

// --- UpdateStatus validation ---

func TestUpdateStatusInvalidType(t *testing.T) {
	svc := newTestService()
	// UpdateStatus validates status_type before touching the DB
	err := svc.UpdateStatus(nil, "1", "invalid_status", "text")
	if err == nil {
		t.Error("UpdateStatus should reject invalid status type")
	}
}

func TestUpdateStatusInvalidUserID(t *testing.T) {
	svc := newTestService()
	err := svc.UpdateStatus(nil, "not-a-number", "online", "text")
	if err == nil {
		t.Error("UpdateStatus should reject non-numeric user ID")
	}
}

// --- RemovePassword validation ---

func TestRemovePasswordInvalidUserID(t *testing.T) {
	svc := newTestService()
	err := svc.RemovePassword(nil, "not-a-number")
	if err == nil {
		t.Error("RemovePassword should reject non-numeric user ID")
	}
}

// --- Register input validation (no DB needed for early returns) ---

func TestRegisterEmptyUsername(t *testing.T) {
	svc := newTestService()
	_, _, err := svc.Register(nil, "", "test@example.com", "", "password123", "")
	if err == nil || err.Error() != "username is required" {
		t.Errorf("expected 'username is required', got: %v", err)
	}
}

func TestRegisterNoEmailOrPhone(t *testing.T) {
	svc := newTestService()
	_, _, err := svc.Register(nil, "testuser", "", "", "password123", "")
	if err == nil || err.Error() != "email or phone number is required" {
		t.Errorf("expected 'email or phone number is required', got: %v", err)
	}
}

func TestRegisterUsernameTooLong(t *testing.T) {
	svc := newTestService()
	longName := "a123456789012345678901234567890ab" // 33 chars
	_, _, err := svc.Register(nil, longName, "test@example.com", "", "password123", "")
	if err == nil || err.Error() != "username must be 32 characters or fewer" {
		t.Errorf("expected username length error, got: %v", err)
	}
}

func TestRegisterInvalidUsernameChars(t *testing.T) {
	svc := newTestService()
	_, _, err := svc.Register(nil, "user name!", "test@example.com", "", "password123", "")
	if err == nil || err.Error() != "username may only contain letters, numbers, underscores, hyphens and dots" {
		t.Errorf("expected invalid username chars error, got: %v", err)
	}
}

func TestRegisterPasswordTooShort(t *testing.T) {
	svc := newTestService()
	_, _, err := svc.Register(nil, "validuser", "test@example.com", "", "short", "")
	if err == nil || err.Error() != "password must be at least 8 characters" {
		t.Errorf("expected password length error, got: %v", err)
	}
}

func TestRegisterPasswordTooLong(t *testing.T) {
	svc := newTestService()
	longPassword := make([]byte, 73)
	for i := range longPassword {
		longPassword[i] = 'a'
	}
	_, _, err := svc.Register(nil, "validuser", "test@example.com", "", string(longPassword), "")
	if err == nil || err.Error() != "password must be 72 characters or fewer (bcrypt limit)" {
		t.Errorf("expected bcrypt limit error, got: %v", err)
	}
}

// --- Login input validation ---

func TestLoginEmptyCredentials(t *testing.T) {
	svc := newTestService()
	_, _, err := svc.Login(nil, "", "password", "")
	if err == nil || err.Error() != "email/phone and password are required" {
		t.Errorf("expected required error, got: %v", err)
	}

	_, _, err = svc.Login(nil, "user@test.com", "", "")
	if err == nil || err.Error() != "email/phone and password are required" {
		t.Errorf("expected required error for empty password, got: %v", err)
	}
}

// --- VerifyEmail validation ---

func TestVerifyEmailEmptyToken(t *testing.T) {
	svc := newTestService()
	err := svc.VerifyEmail(nil, "")
	if err == nil || err.Error() != "verification token is required" {
		t.Errorf("expected 'verification token is required', got: %v", err)
	}
}

// --- ResetPassword validation ---

func TestResetPasswordEmptyToken(t *testing.T) {
	svc := newTestService()
	err := svc.ResetPassword(nil, "", "newpassword123")
	if err == nil || err.Error() != "reset token is required" {
		t.Errorf("expected 'reset token is required', got: %v", err)
	}
}

func TestResetPasswordTooShort(t *testing.T) {
	svc := newTestService()
	err := svc.ResetPassword(nil, "sometoken", "short")
	if err == nil || err.Error() != "password must be at least 8 characters" {
		t.Errorf("expected password length error, got: %v", err)
	}
}

func TestResetPasswordTooLong(t *testing.T) {
	svc := newTestService()
	longPassword := make([]byte, 73)
	for i := range longPassword {
		longPassword[i] = 'a'
	}
	err := svc.ResetPassword(nil, "sometoken", string(longPassword))
	if err == nil || err.Error() != "password must be 72 characters or fewer (bcrypt limit)" {
		t.Errorf("expected bcrypt limit error, got: %v", err)
	}
}

// --- ChangeEmail validation ---

func TestChangeEmailEmptyEmail(t *testing.T) {
	svc := newTestService()
	_, err := svc.ChangeEmail(nil, "1", "", "password")
	if err == nil || err.Error() != "new email is required" {
		t.Errorf("expected 'new email is required', got: %v", err)
	}
}

func TestChangeEmailEmptyPassword(t *testing.T) {
	svc := newTestService()
	_, err := svc.ChangeEmail(nil, "1", "new@example.com", "")
	if err == nil || err.Error() != "password is required to change email" {
		t.Errorf("expected password required error, got: %v", err)
	}
}

func TestChangeEmailInvalidUserID(t *testing.T) {
	svc := newTestService()
	_, err := svc.ChangeEmail(nil, "not-a-number", "new@example.com", "password")
	if err == nil || err.Error() != "invalid user ID" {
		t.Errorf("expected 'invalid user ID', got: %v", err)
	}
}

// --- ChangePhone validation ---

func TestChangePhoneEmptyPhone(t *testing.T) {
	svc := newTestService()
	_, err := svc.ChangePhone(nil, "1", "", "password")
	if err == nil || err.Error() != "phone number is required" {
		t.Errorf("expected 'phone number is required', got: %v", err)
	}
}

func TestChangePhoneEmptyPassword(t *testing.T) {
	svc := newTestService()
	_, err := svc.ChangePhone(nil, "1", "+1234567890", "")
	if err == nil || err.Error() != "password is required to change phone" {
		t.Errorf("expected password required error, got: %v", err)
	}
}

func TestChangePhoneInvalidUserID(t *testing.T) {
	svc := newTestService()
	_, err := svc.ChangePhone(nil, "abc", "+1234567890", "password")
	if err == nil || err.Error() != "invalid user ID" {
		t.Errorf("expected 'invalid user ID', got: %v", err)
	}
}

// --- ResendVerification validation ---

func TestResendVerificationInvalidUserID(t *testing.T) {
	svc := newTestService()
	err := svc.ResendVerification(nil, "not-a-number")
	if err == nil || err.Error() != "invalid user ID" {
		t.Errorf("expected 'invalid user ID', got: %v", err)
	}
}

// --- GetUserByID validation ---

func TestGetUserByIDInvalidUserID(t *testing.T) {
	svc := newTestService()
	_, err := svc.GetUserByID(nil, "not-a-number")
	if err == nil || err.Error() != "invalid user ID" {
		t.Errorf("expected 'invalid user ID', got: %v", err)
	}
}

// --- SendPhoneVerification validation ---

func TestSendPhoneVerificationInvalidUserID(t *testing.T) {
	svc := newTestService()
	err := svc.SendPhoneVerification(nil, "abc")
	if err == nil || err.Error() != "invalid user ID" {
		t.Errorf("expected 'invalid user ID', got: %v", err)
	}
}

// --- VerifyPhone validation ---

func TestVerifyPhoneInvalidUserID(t *testing.T) {
	svc := newTestService()
	err := svc.VerifyPhone(nil, "abc", "123456")
	if err == nil || err.Error() != "invalid user ID" {
		t.Errorf("expected 'invalid user ID', got: %v", err)
	}
}

// NOTE: The following service methods require a real database connection
// and should be tested with dockertest integration tests:
//   - Register (full flow with DB writes)
//   - Login (full flow with DB reads)
//   - UpdateProfile (full flow with DB reads/writes)
//   - VerifyEmail (DB lookup by token)
//   - ResendVerification (full flow)
//   - ChangeEmail (full flow)
//   - ChangePhone (full flow)
//   - RequestPasswordReset (full flow)
//   - ResetPassword (full flow with DB token consumption)
//   - GetUserByID (DB lookup)
//   - IsForceLoggedOut (DB query)
//   - SendPhoneVerification (full flow)
//   - VerifyPhone (full flow)
