package passkey

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"parley/internal/db"
)

const sessionTTL = 5 * time.Minute
const sessionKeyPrefix = "webauthn:session:"

// Service handles passkey registration and authentication.
type Service struct {
	wauth *webauthn.WebAuthn
	rdb   *goredis.Client
	repo  *db.Repository
}

// PasskeyInfo is the public representation of a stored passkey.
type PasskeyInfo struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	CreatedAt string  `json:"created_at"`
	LastUsed  *string `json:"last_used,omitempty"`
}

// passkeyUser implements webauthn.User.
type passkeyUser struct {
	id          []byte
	username    string
	displayName string
	credentials []webauthn.Credential
}

func (u *passkeyUser) WebAuthnID() []byte                         { return u.id }
func (u *passkeyUser) WebAuthnName() string                       { return u.username }
func (u *passkeyUser) WebAuthnDisplayName() string                { return u.displayName }
func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

// New creates a new passkey Service. Returns nil if WebAuthn configuration fails.
func New(repo *db.Repository, rdb *goredis.Client, siteURL string) *Service {
	rpid := rpidFromURL(siteURL)
	origins := originsFromURL(siteURL)
	wauth, err := webauthn.New(&webauthn.Config{
		RPID:          rpid,
		RPDisplayName: "Parley",
		RPOrigins:     origins,
	})
	if err != nil {
		log.Printf("passkey: failed to configure WebAuthn (RPID=%s): %v", rpid, err)
		return nil
	}
	log.Printf("passkey: WebAuthn configured (RPID=%s, origins=%v)", rpid, origins)
	return &Service{wauth: wauth, rdb: rdb, repo: repo}
}

func rpidFromURL(siteURL string) string {
	u, err := url.Parse(siteURL)
	if err != nil || u.Host == "" {
		return "localhost"
	}
	return u.Hostname()
}

func originsFromURL(siteURL string) []string {
	origins := []string{siteURL}
	// Only accept dev-server origins when explicitly running in dev mode.
	// Production deployments must leave PARLEY_ENV unset (or set to anything else)
	// so WebAuthn rejects assertions forged against a localhost RP origin.
	if os.Getenv("PARLEY_ENV") == "dev" {
		origins = append(origins, "http://localhost:5173", "http://localhost:8080")
	}
	return origins
}

func (s *Service) saveSession(ctx context.Context, data *webauthn.SessionData) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	sessionID := hex.EncodeToString(b)
	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	if err := s.rdb.Set(ctx, sessionKeyPrefix+sessionID, raw, sessionTTL).Err(); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (s *Service) loadAndDeleteSession(ctx context.Context, sessionID string) (*webauthn.SessionData, error) {
	raw, err := s.rdb.GetDel(ctx, sessionKeyPrefix+sessionID).Bytes()
	if err == goredis.Nil {
		return nil, fmt.Errorf("session not found or expired")
	}
	if err != nil {
		return nil, err
	}
	var data webauthn.SessionData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func buildUser(dbUser *db.User, pks []db.Passkey) *passkeyUser {
	creds := make([]webauthn.Credential, len(pks))
	for i, pk := range pks {
		creds[i] = webauthn.Credential{
			ID:        pk.CredentialID,
			PublicKey: pk.PublicKey,
			Authenticator: webauthn.Authenticator{
				AAGUID:    pk.AAGUID,
				SignCount: uint32(pk.SignCount),
			},
			Flags: webauthn.CredentialFlags{
				BackupEligible: pk.BackupEligible,
				BackupState:    pk.BackupState,
			},
		}
	}
	dn := dbUser.DisplayName
	if dn == "" {
		dn = dbUser.Username
	}
	// Use email as the account identifier (shown in browser passkey picker).
	// Fall back to phone, then username if neither is set.
	name := dbUser.Email
	if name == "" {
		name = dbUser.PhoneNumber
	}
	if name == "" {
		name = dbUser.Username
	}
	return &passkeyUser{
		id:          []byte(strconv.FormatInt(dbUser.ID, 10)),
		username:    name,
		displayName: dn,
		credentials: creds,
	}
}

// RegisterBegin starts passkey registration for an authenticated user.
// Returns the creation options JSON and a session ID.
func (s *Service) RegisterBegin(ctx context.Context, userIDStr string) (*protocol.CredentialCreation, string, error) {
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return nil, "", fmt.Errorf("invalid user ID")
	}
	dbUser, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, "", fmt.Errorf("user not found")
	}
	pks, err := s.repo.GetPasskeysByUserID(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	user := buildUser(dbUser, pks)

	excludeList := make([]protocol.CredentialDescriptor, len(pks))
	for i, pk := range pks {
		excludeList[i] = protocol.CredentialDescriptor{
			Type:         protocol.PublicKeyCredentialType,
			CredentialID: pk.CredentialID,
		}
	}

	creation, session, err := s.wauth.BeginRegistration(user,
		webauthn.WithExclusions(excludeList),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementPreferred),
	)
	if err != nil {
		return nil, "", err
	}
	sessionID, err := s.saveSession(ctx, session)
	if err != nil {
		return nil, "", err
	}
	return creation, sessionID, nil
}

// RegisterFinish completes passkey registration. The credentialBody is the raw
// JSON body containing the PublicKeyCredential from the browser.
func (s *Service) RegisterFinish(ctx context.Context, userIDStr, sessionID, name string, credentialBody []byte) error {
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid user ID")
	}
	session, err := s.loadAndDeleteSession(ctx, sessionID)
	if err != nil {
		return err
	}
	dbUser, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("user not found")
	}
	pks, _ := s.repo.GetPasskeysByUserID(ctx, userID)
	user := buildUser(dbUser, pks)

	fakeReq := &http.Request{
		Body: io.NopCloser(bytes.NewReader(credentialBody)),
	}
	credential, err := s.wauth.FinishRegistration(user, *session, fakeReq)
	if err != nil {
		return err
	}
	if name == "" {
		name = "Passkey"
	}
	pk := &db.Passkey{
		UserID:         userID,
		CredentialID:   credential.ID,
		PublicKey:      credential.PublicKey,
		SignCount:      int64(credential.Authenticator.SignCount),
		AAGUID:         credential.Authenticator.AAGUID,
		Name:           name,
		BackupEligible: credential.Flags.BackupEligible,
		BackupState:    credential.Flags.BackupState,
	}
	return s.repo.CreatePasskey(ctx, pk)
}

// LoginBegin starts discoverable passkey login (no email required).
func (s *Service) LoginBegin(ctx context.Context) (*protocol.CredentialAssertion, string, error) {
	assertion, session, err := s.wauth.BeginDiscoverableLogin()
	if err != nil {
		return nil, "", err
	}
	sessionID, err := s.saveSession(ctx, session)
	if err != nil {
		return nil, "", err
	}
	return assertion, sessionID, nil
}

// LoginFinish completes discoverable passkey login. Returns the userID string on success.
func (s *Service) LoginFinish(ctx context.Context, sessionID string, credentialBody []byte) (string, error) {
	session, err := s.loadAndDeleteSession(ctx, sessionID)
	if err != nil {
		return "", err
	}

	var resolvedUserIDStr string
	fakeReq := &http.Request{
		Body: io.NopCloser(bytes.NewReader(credentialBody)),
	}
	credential, err := s.wauth.FinishDiscoverableLogin(
		func(rawID, userHandle []byte) (webauthn.User, error) {
			pk, err := s.repo.GetPasskeyByCredentialID(ctx, rawID)
			if err != nil {
				return nil, fmt.Errorf("passkey not found")
			}
			dbUser, err := s.repo.GetUserByID(ctx, pk.UserID)
			if err != nil {
				return nil, fmt.Errorf("user not found")
			}
			resolvedUserIDStr = strconv.FormatInt(dbUser.ID, 10)
			pks, _ := s.repo.GetPasskeysByUserID(ctx, dbUser.ID)
			return buildUser(dbUser, pks), nil
		},
		*session,
		fakeReq,
	)
	if err != nil {
		return "", err
	}

	// Update sign count and last used timestamp
	pk, lookupErr := s.repo.GetPasskeyByCredentialID(ctx, credential.ID)
	if lookupErr == nil && pk != nil {
		_ = s.repo.UpdatePasskeySignCount(ctx, pk.ID, int64(credential.Authenticator.SignCount))
	}
	return resolvedUserIDStr, nil
}

// List returns all passkeys for a user.
func (s *Service) List(ctx context.Context, userIDStr string) ([]PasskeyInfo, error) {
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID")
	}
	pks, err := s.repo.GetPasskeysByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]PasskeyInfo, len(pks))
	for i, pk := range pks {
		info := PasskeyInfo{
			ID:        pk.ID,
			Name:      pk.Name,
			CreatedAt: pk.CreatedAt.Format(time.RFC3339),
		}
		if pk.LastUsedAt != nil {
			t := pk.LastUsedAt.Format(time.RFC3339)
			info.LastUsed = &t
		}
		out[i] = info
	}
	return out, nil
}

// Delete removes a passkey owned by the given user.
// Rejects if this is the user's last passkey and they have no password set.
func (s *Service) Delete(ctx context.Context, userIDStr, idStr string) error {
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid user ID")
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid passkey ID")
	}

	// Guard: refuse to remove the last passkey if no password is set.
	pks, err := s.repo.GetPasskeysByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if len(pks) <= 1 {
		dbUser, err := s.repo.GetUserByID(ctx, userID)
		if err != nil {
			return err
		}
		if dbUser.PasswordHash == "!" {
			return fmt.Errorf("cannot remove your only passkey without a password set")
		}
	}

	return s.repo.DeletePasskey(ctx, id, userID)
}

// Rename updates the display name of a passkey.
func (s *Service) Rename(ctx context.Context, userIDStr, idStr, name string) error {
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid user ID")
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid passkey ID")
	}
	if name == "" {
		return fmt.Errorf("name is required")
	}
	return s.repo.RenamePasskey(ctx, id, userID, name)
}
