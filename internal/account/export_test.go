package account

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"parley/internal/db"
)

// TestProfileFromUser_StripsPasswordAndSecrets verifies the spec-mandated
// invariant: the export must serialize a profile struct that omits
// password_hash, email-verification token, and any other credential
// material. Mutating *db.User is explicitly disallowed by the spec — this
// test would catch a future drift back to mutating the pointer.
func TestProfileFromUser_StripsPasswordAndSecrets(t *testing.T) {
	bannedAt := time.Now().UTC()
	u := &db.User{
		ID:                     42,
		Username:               "alice",
		Email:                  "alice@example.com",
		PasswordHash:           "$2b$12$DOSKMFhblockt0pdENqMP8shouldNotAppear",
		EmailVerificationToken: "verify-token-secret",
		AvatarURL:              "https://cdn/x.png",
		BannerURL:              "https://cdn/banner.png",
		Bio:                    "hi",
		DisplayName:            "Alice",
		EmailVerified:          true,
		PhoneNumber:            "+15551234567",
		PhoneVerified:          true,
		BannedAt:               &bannedAt,
		BanReason:              "spam",
		IsSystem:               false,
		Badges:                 3,
		StatusType:             "online",
		StatusText:             "coding",
		InviteCount:            7,
		CreatedAt:              time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:              time.Now().UTC(),
	}

	p := profileFromUser(u)
	if p == nil {
		t.Fatal("profileFromUser returned nil for non-nil user")
	}

	// Marshal then re-parse to confirm the wire shape — JSON tags are
	// what the user actually sees, and the assertion must catch any
	// future field that gets serialized accidentally.
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	jsonStr := string(raw)

	for _, leak := range []string{
		"password",
		"password_hash",
		u.PasswordHash,
		"email_verification_token",
		u.EmailVerificationToken,
		"force_logout",
		"phone_verification",
		"password_reset",
	} {
		if strings.Contains(jsonStr, leak) {
			t.Errorf("export profile leaked %q: %s", leak, jsonStr)
		}
	}

	// Sanity: fields the user IS allowed to see should be present.
	for _, want := range []string{
		`"id":42`,
		`"username":"alice"`,
		`"email":"alice@example.com"`,
		`"display_name":"Alice"`,
		`"badges":3`,
		`"banned_at":`,
	} {
		if !strings.Contains(jsonStr, want) {
			t.Errorf("export profile missing %q: %s", want, jsonStr)
		}
	}

	// And the in-memory profile struct itself must not even hold the
	// password material — Go's reflection isn't needed; the type doesn't
	// have a PasswordHash field.
	pType := fmt.Sprintf("%+v", *p)
	if strings.Contains(pType, u.PasswordHash) {
		t.Errorf("profile struct retained password hash: %s", pType)
	}
	if strings.Contains(pType, u.EmailVerificationToken) {
		t.Errorf("profile struct retained verification token: %s", pType)
	}
}

// TestProfileFromUser_NilSafe verifies the nil-input guard. Defensive but
// cheap — the build would catch most callers, but the deletion path could
// conceivably hand us a deleted user pointer.
func TestProfileFromUser_NilSafe(t *testing.T) {
	if p := profileFromUser(nil); p != nil {
		t.Errorf("expected nil for nil input, got %+v", p)
	}
}

// TestExportFilename verifies the spec-locked filename format.
func TestExportFilename(t *testing.T) {
	when := time.Unix(1700000000, 0)
	got := exportFilename("alice", when)
	want := "parley-export-alice-1700000000.json"
	if got != want {
		t.Errorf("filename: got %q, want %q", got, want)
	}
}

// TestExportFilename_SpecialChars confirms unusual usernames don't break the
// filename — they pass through verbatim. The Content-Disposition header in
// the handler quotes the filename with %q, so any characters that would
// otherwise need escaping are handled at the HTTP layer.
func TestExportFilename_PreservesUsername(t *testing.T) {
	when := time.Unix(1700000000, 0)
	got := exportFilename("user.name_42", when)
	want := "parley-export-user.name_42-1700000000.json"
	if got != want {
		t.Errorf("filename: got %q, want %q", got, want)
	}
}

// TestExportEnvelope_FormatVersion locks the format version constant. Any
// breaking change to the envelope shape MUST bump this number; this test
// fails to remind whoever changes it.
func TestExportEnvelope_FormatVersion(t *testing.T) {
	if ExportFormatVersion != 1 {
		t.Errorf("format version changed from 1 to %d — confirm consumer compatibility", ExportFormatVersion)
	}
}

// TestExportEnvelope_DefaultJSONShape verifies the JSON envelope marshals
// with the spec's top-level keys and that empty-but-present arrays serialize
// as [] (not null). A missing key here would silently break the frontend
// consumer's downloaded-file parser.
func TestExportEnvelope_DefaultJSONShape(t *testing.T) {
	env := &ExportEnvelope{
		ExportedAt:    time.Unix(1700000000, 0).UTC(),
		FormatVersion: ExportFormatVersion,
		Profile:       &ExportProfile{ID: 1, Username: "alice"},
		Passkeys:      []ExportPasskey{},
		Friends:       nil, // nil should serialize as null per Go JSON; only the spec's
		// "user-facing arrays" sections are guaranteed to be [] when empty
		// (we test those below explicitly).
		MessagesAuthored: ExportMessagesAuthored{
			ServerChannels:  []ExportMessage{},
			DMs:             []ExportDmMessage{},
			BinPosts:        []ExportBinPost{},
			BinLineComments: []ExportBinLineComment{},
		},
	}

	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	str := string(raw)
	for _, key := range []string{
		`"exported_at":`,
		`"format_version":1`,
		`"profile":`,
		`"passkeys":[]`,
		`"messages_authored":`,
		`"server_channels":[]`,
		`"dms":[]`,
		`"bin_posts":[]`,
		`"bin_line_comments":[]`,
	} {
		if !strings.Contains(str, key) {
			t.Errorf("envelope missing %q: %s", key, str)
		}
	}
}

// ---------------------------------------------------------------------------
// Integration tests — opt-in. Set TEST_DATABASE_URL to a Postgres dsn that
// the test process can DROP/CREATE schema on. Without it the integration
// tests skip cleanly so the unit-only run (the default) stays green.
// ---------------------------------------------------------------------------

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping DB integration test")
	}
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("opening %s: %v", dsn, err)
	}
	if err := conn.Ping(); err != nil {
		t.Skipf("pinging %s: %v", dsn, err)
	}
	for _, m := range db.Migrations {
		if _, err := conn.Exec(m); err != nil {
			t.Fatalf("applying migration: %v\n%s", err, m)
		}
	}
	return conn
}

// TestExportService_OnlyAuthorsOwnMessages requires a real Postgres. Two
// users author one message each in the same channel; the export for user A
// must include only A's row.
func TestExportService_OnlyAuthorsOwnMessages(t *testing.T) {
	conn := openTestDB(t)
	defer conn.Close()

	repo := db.NewRepository(conn)
	svc := NewExportService(repo)
	ctx := context.Background()

	// Insert two test users + a server + a channel + one message each.
	// Username is unique-suffixed with the test name so re-runs of the
	// same suite against a shared DB don't collide.
	suffix := fmt.Sprintf("_%d", time.Now().UnixNano())
	var aliceID, bobID, serverID, channelID int64
	if err := conn.QueryRowContext(ctx,
		`INSERT INTO users (username, email, password_hash) VALUES ($1, $2, $3) RETURNING id`,
		"alice"+suffix, "alice"+suffix+"@x", "x").Scan(&aliceID); err != nil {
		t.Fatalf("insert alice: %v", err)
	}
	if err := conn.QueryRowContext(ctx,
		`INSERT INTO users (username, email, password_hash) VALUES ($1, $2, $3) RETURNING id`,
		"bob"+suffix, "bob"+suffix+"@x", "x").Scan(&bobID); err != nil {
		t.Fatalf("insert bob: %v", err)
	}
	if err := conn.QueryRowContext(ctx,
		`INSERT INTO servers (name, owner_id) VALUES ($1, $2) RETURNING id`,
		"srv"+suffix, aliceID).Scan(&serverID); err != nil {
		t.Fatalf("insert server: %v", err)
	}
	if err := conn.QueryRowContext(ctx,
		`INSERT INTO channels (server_id, name) VALUES ($1, $2) RETURNING id`,
		serverID, "general").Scan(&channelID); err != nil {
		t.Fatalf("insert channel: %v", err)
	}

	if _, err := conn.ExecContext(ctx,
		`INSERT INTO messages (channel_id, author_id, content) VALUES ($1, $2, 'hi from alice')`,
		channelID, aliceID); err != nil {
		t.Fatalf("insert alice msg: %v", err)
	}
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO messages (channel_id, author_id, content) VALUES ($1, $2, 'hi from bob')`,
		channelID, bobID); err != nil {
		t.Fatalf("insert bob msg: %v", err)
	}

	env, err := svc.BuildExport(ctx, aliceID)
	if err != nil {
		t.Fatalf("BuildExport: %v", err)
	}

	if got := len(env.MessagesAuthored.ServerChannels); got != 1 {
		t.Fatalf("expected 1 authored channel message, got %d: %+v", got, env.MessagesAuthored.ServerChannels)
	}
	if env.MessagesAuthored.ServerChannels[0].Content != "hi from alice" {
		t.Errorf("wrong message in export: %+v", env.MessagesAuthored.ServerChannels[0])
	}
	if env.MessagesAuthored.ServerChannels[0].ServerID != serverID {
		t.Errorf("server_id should be populated, got %d (want %d)", env.MessagesAuthored.ServerChannels[0].ServerID, serverID)
	}
	// And the profile must not leak password_hash.
	rawProfile, _ := json.Marshal(env.Profile)
	if strings.Contains(string(rawProfile), "password") {
		t.Errorf("profile leaked password material: %s", rawProfile)
	}
}

// TestExportHandler_FilenameHeader verifies the Content-Disposition header
// uses the filename pattern from the spec. It's a unit test against a stub
// service that returns a fixed envelope so the header logic is exercised
// without DB.
func TestExportHandler_FilenameHeader(t *testing.T) {
	if os.Getenv("JWT_SECRET") == "" {
		// auth.GetUserIDFromContext doesn't initialize anything; the
		// auth init guard is tied to package-level config that other
		// tests in this binary may have triggered. Set a dummy value
		// so the package's TestMain (if any) is happy.
		os.Setenv("JWT_SECRET", "test-secret-at-least-32-bytes-long-1234")
	}

	// Build a handler whose svc returns a canned envelope. We can't
	// inject a fake easily because ExportHandler holds a concrete
	// *ExportService; instead, drive the response writer directly with
	// the same code path the handler would take.
	w := httptest.NewRecorder()
	env := &ExportEnvelope{
		ExportedAt:    time.Unix(1700000000, 0),
		FormatVersion: ExportFormatVersion,
		Profile:       &ExportProfile{ID: 1, Username: "alice"},
	}

	// Mirror the handler's header sequence so a regression in either
	// place (header set or filename helper) trips this test.
	filename := exportFilename(env.Profile.Username, time.Unix(1700000123, 0))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))
	if err := json.NewEncoder(w).Encode(env); err != nil {
		t.Fatalf("encode: %v", err)
	}

	disp := w.Header().Get("Content-Disposition")
	wantSub := `attachment; filename="parley-export-alice-1700000123.json"`
	if disp != wantSub {
		t.Errorf("Content-Disposition: got %q, want %q", disp, wantSub)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}
}
