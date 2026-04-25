package auth

import (
	"log"
	"os"
	"time"
)

// Config holds JWT configuration settings.
//
// SecretKey signs + verifies normal user session JWTs.
//
// ImpersonationSecretKey is a SEPARATE signing key used only to verify admin
// impersonation tokens (see F-admin-jwt-secret). The admin service holds this
// key and mints tokens with it; the api holds it only to verify. Keeping the
// two keys separate means a compromised admin container cannot mint regular
// user JWTs — only short-lived impersonation tokens, which `denyImpersonation`
// middleware blocks from sensitive routes.
//
// ImpersonationSecretKey is optional on the api: deployments that never run
// an admin panel can leave IMPERSONATION_JWT_SECRET unset. In that mode the
// validator rejects any token carrying an `impersonation: true` claim with
// "impersonation unavailable" rather than silently treating it as a normal
// session.
type Config struct {
	SecretKey              string
	ImpersonationSecretKey string
	TokenExpiry            time.Duration
}

// DefaultConfig returns the default JWT configuration
func DefaultConfig() *Config {
	secretKey := os.Getenv("JWT_SECRET")
	if secretKey == "" {
		log.Fatal("JWT_SECRET environment variable is not set — refusing to start with an insecure default")
	}

	// Optional: api nodes without an admin panel don't need to verify
	// impersonation tokens at all.
	impersonationKey := os.Getenv("IMPERSONATION_JWT_SECRET")

	// JWT_EXPIRY accepts any time.ParseDuration string ("720h", "30d" is NOT
	// supported by stdlib — use hours). Default is 30 days for a user-friendly
	// session length; the force_logout_at mechanism (isUserForceLoggedOut)
	// remains the revocation path for compromised accounts.
	tokenExpiry := 30 * 24 * time.Hour
	if v := os.Getenv("JWT_EXPIRY"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			tokenExpiry = d
		} else {
			log.Printf("auth: ignoring invalid JWT_EXPIRY=%q (want e.g. 720h); using default %s", v, tokenExpiry)
		}
	}

	return &Config{
		SecretKey:              secretKey,
		ImpersonationSecretKey: impersonationKey,
		TokenExpiry:            tokenExpiry,
	}
}

// GetConfig returns the global config instance
func GetConfig() *Config {
	return defaultConfig
}

var defaultConfig = DefaultConfig()