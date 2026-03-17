package db

// Migrations contains all database migration statements
var Migrations = []string{
	`-- Create users table
	CREATE TABLE IF NOT EXISTS users (
		id BIGSERIAL PRIMARY KEY,
		username VARCHAR(255) NOT NULL UNIQUE,
		email VARCHAR(255) NOT NULL UNIQUE,
		password_hash VARCHAR(255) NOT NULL,
		avatar_url TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	-- Create servers table
	CREATE TABLE IF NOT EXISTS servers (
		id BIGSERIAL PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		icon_url TEXT,
		owner_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	-- Create server_members table
	CREATE TABLE IF NOT EXISTS server_members (
		id BIGSERIAL PRIMARY KEY,
		server_id BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
		user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		nickname VARCHAR(255) NOT NULL DEFAULT '',
		joined_at TIMESTAMP NOT NULL DEFAULT NOW(),
		UNIQUE(server_id, user_id)
	);

	-- Create channels table
	CREATE TABLE IF NOT EXISTS channels (
		id BIGSERIAL PRIMARY KEY,
		server_id BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
		name VARCHAR(255) NOT NULL,
		channel_type INTEGER NOT NULL DEFAULT 0,
		position INTEGER NOT NULL DEFAULT 0,
		parent_id BIGINT REFERENCES channels(id) ON DELETE SET NULL,
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	-- Create messages table
	CREATE TABLE IF NOT EXISTS messages (
		id BIGSERIAL PRIMARY KEY,
		channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
		author_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		content TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	-- Create indexes for better query performance
	CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
	CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
	CREATE INDEX IF NOT EXISTS idx_servers_owner_id ON servers(owner_id);
	CREATE INDEX IF NOT EXISTS idx_server_members_server_id ON server_members(server_id);
	CREATE INDEX IF NOT EXISTS idx_server_members_user_id ON server_members(user_id);
	CREATE INDEX IF NOT EXISTS idx_channels_server_id ON channels(server_id);
	CREATE INDEX IF NOT EXISTS idx_channels_parent_id ON channels(parent_id);
	CREATE INDEX IF NOT EXISTS idx_messages_channel_id ON messages(channel_id);
	CREATE INDEX IF NOT EXISTS idx_messages_author_id ON messages(author_id);
	CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at);
	`,

	`-- Create DM channels table
	CREATE TABLE IF NOT EXISTS dm_channels (
		id BIGSERIAL PRIMARY KEY,
		user1_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		user2_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		UNIQUE(user1_id, user2_id)
	);

	-- Create DM messages table
	CREATE TABLE IF NOT EXISTS dm_messages (
		id BIGSERIAL PRIMARY KEY,
		dm_channel_id BIGINT NOT NULL REFERENCES dm_channels(id) ON DELETE CASCADE,
		author_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		content TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	-- Create indexes for DM tables
	CREATE INDEX IF NOT EXISTS idx_dm_channels_user1 ON dm_channels(user1_id);
	CREATE INDEX IF NOT EXISTS idx_dm_channels_user2 ON dm_channels(user2_id);
	CREATE INDEX IF NOT EXISTS idx_dm_messages_channel ON dm_messages(dm_channel_id);
	CREATE INDEX IF NOT EXISTS idx_dm_messages_created ON dm_messages(created_at);
	`,

	`-- Create invites table
	CREATE TABLE IF NOT EXISTS invites (
		id BIGSERIAL PRIMARY KEY,
		server_id BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
		code VARCHAR(8) NOT NULL UNIQUE,
		created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_invites_code ON invites(code);
	CREATE INDEX IF NOT EXISTS idx_invites_server_id ON invites(server_id);
	`,

	`-- Add vanity_url to servers table
	ALTER TABLE servers ADD COLUMN IF NOT EXISTS vanity_url VARCHAR(64) UNIQUE;
	CREATE INDEX IF NOT EXISTS idx_servers_vanity_url ON servers(vanity_url) WHERE vanity_url IS NOT NULL;
	`,

	`-- Add updated_at to channels table
	ALTER TABLE channels ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP NOT NULL DEFAULT NOW();
	`,

	`-- Create message_reactions table
	CREATE TABLE IF NOT EXISTS message_reactions (
		id BIGSERIAL PRIMARY KEY,
		message_id BIGINT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
		user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		emoji VARCHAR(64) NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		UNIQUE(message_id, user_id, emoji)
	);

	CREATE INDEX IF NOT EXISTS idx_message_reactions_message_id ON message_reactions(message_id);
	`,

	`-- Add nonce to messages for client-side deduplication
	ALTER TABLE messages ADD COLUMN IF NOT EXISTS nonce VARCHAR(64);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_nonce ON messages(nonce) WHERE nonce IS NOT NULL AND nonce != '';
	`,

	`-- Add file attachment support
ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS attachment_url  TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS attachment_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS attachment_type TEXT NOT NULL DEFAULT '';

ALTER TABLE dm_messages
    ADD COLUMN IF NOT EXISTS attachment_url  TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS attachment_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS attachment_type TEXT NOT NULL DEFAULT '';

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS avatar_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS banner_url TEXT NOT NULL DEFAULT '';
`,

	`-- Create server roles tables
CREATE TABLE IF NOT EXISTS server_roles (
    id BIGSERIAL PRIMARY KEY,
    server_id BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    color VARCHAR(7) NOT NULL DEFAULT '#99aab5',
    permissions BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(server_id, name)
);

CREATE TABLE IF NOT EXISTS server_member_roles (
    server_id BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id BIGINT NOT NULL REFERENCES server_roles(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (server_id, user_id, role_id)
);

CREATE INDEX IF NOT EXISTS idx_server_roles_server_id ON server_roles(server_id);
CREATE INDEX IF NOT EXISTS idx_server_member_roles_server_user ON server_member_roles(server_id, user_id);
CREATE INDEX IF NOT EXISTS idx_server_member_roles_role ON server_member_roles(role_id);
`,

	`-- Add email verification support
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verification_token VARCHAR(64);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_verification_token ON users(email_verification_token) WHERE email_verification_token IS NOT NULL;
`,

	`-- Add email rate-limit counters
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_resend_count INT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_resend_date DATE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_change_count INT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_change_date DATE;
`,

	`-- Add phone verification support and make email optional
ALTER TABLE users ALTER COLUMN email DROP NOT NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS phone_number VARCHAR(20);
ALTER TABLE users ADD COLUMN IF NOT EXISTS phone_verified BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS phone_verification_code VARCHAR(6);
ALTER TABLE users ADD COLUMN IF NOT EXISTS phone_code_expires_at TIMESTAMP;
ALTER TABLE users ADD COLUMN IF NOT EXISTS sms_resend_count INT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS sms_resend_date DATE;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_phone ON users(phone_number) WHERE phone_number IS NOT NULL;
`,

	`-- Admin users table and admin-related user columns
ALTER TABLE users ADD COLUMN IF NOT EXISTS banned_at TIMESTAMP;
ALTER TABLE users ADD COLUMN IF NOT EXISTS ban_reason TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS force_logout_at TIMESTAMP;
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_system BOOLEAN NOT NULL DEFAULT FALSE;

-- Seed system bot user (used for ToS DMs on server disband)
INSERT INTO users (username, email, password_hash, is_system, created_at, updated_at)
VALUES ('Parley', NULL, 'SYSTEM_ACCOUNT_NOT_FOR_LOGIN', TRUE, NOW(), NOW())
ON CONFLICT (username) DO NOTHING;

CREATE TABLE IF NOT EXISTS admin_users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    last_login_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS report_categories (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Default categories
INSERT INTO report_categories (name) VALUES
    ('Spam'),
    ('Harassment'),
    ('Hate Speech'),
    ('NSFW Content'),
    ('Impersonation'),
    ('Misinformation'),
    ('Other')
ON CONFLICT (name) DO NOTHING;

CREATE TABLE IF NOT EXISTS reports (
    id BIGSERIAL PRIMARY KEY,
    reporter_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reported_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reported_message_id BIGINT REFERENCES messages(id) ON DELETE SET NULL,
    category_id BIGINT NOT NULL REFERENCES report_categories(id),
    description TEXT NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'open',
    resolved_by BIGINT REFERENCES admin_users(id) ON DELETE SET NULL,
    resolution_note TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_reports_status ON reports(status);
CREATE INDEX IF NOT EXISTS idx_reports_reporter ON reports(reporter_id);
CREATE INDEX IF NOT EXISTS idx_reports_reported_user ON reports(reported_user_id);
CREATE INDEX IF NOT EXISTS idx_reports_category ON reports(category_id);
`,

	`-- Add topic to channels table
ALTER TABLE channels ADD COLUMN IF NOT EXISTS topic TEXT NOT NULL DEFAULT '';
`,

	`-- Random fixed-length ID generators
-- Users: 12-digit (100000000000–999999999999), not in URLs so can be longer
-- Servers, channels, dm_channels: 9-digit (100000000–999999999), appear in URLs

CREATE OR REPLACE FUNCTION gen_user_id() RETURNS BIGINT LANGUAGE plpgsql VOLATILE AS $$
DECLARE new_id BIGINT;
BEGIN
  LOOP
    new_id := floor(random() * 900000000000 + 100000000000)::BIGINT;
    EXIT WHEN NOT EXISTS (SELECT 1 FROM users WHERE id = new_id);
  END LOOP;
  RETURN new_id;
END; $$;

CREATE OR REPLACE FUNCTION gen_server_id() RETURNS BIGINT LANGUAGE plpgsql VOLATILE AS $$
DECLARE new_id BIGINT;
BEGIN
  LOOP
    new_id := floor(random() * 900000000 + 100000000)::BIGINT;
    EXIT WHEN NOT EXISTS (SELECT 1 FROM servers WHERE id = new_id);
  END LOOP;
  RETURN new_id;
END; $$;

CREATE OR REPLACE FUNCTION gen_channel_id() RETURNS BIGINT LANGUAGE plpgsql VOLATILE AS $$
DECLARE new_id BIGINT;
BEGIN
  LOOP
    new_id := floor(random() * 900000000 + 100000000)::BIGINT;
    EXIT WHEN NOT EXISTS (SELECT 1 FROM channels WHERE id = new_id);
  END LOOP;
  RETURN new_id;
END; $$;

CREATE OR REPLACE FUNCTION gen_dm_channel_id() RETURNS BIGINT LANGUAGE plpgsql VOLATILE AS $$
DECLARE new_id BIGINT;
BEGIN
  LOOP
    new_id := floor(random() * 900000000 + 100000000)::BIGINT;
    EXIT WHEN NOT EXISTS (SELECT 1 FROM dm_channels WHERE id = new_id);
  END LOOP;
  RETURN new_id;
END; $$;

ALTER TABLE users      ALTER COLUMN id SET DEFAULT gen_user_id();
ALTER TABLE servers    ALTER COLUMN id SET DEFAULT gen_server_id();
ALTER TABLE channels   ALTER COLUMN id SET DEFAULT gen_channel_id();
ALTER TABLE dm_channels ALTER COLUMN id SET DEFAULT gen_dm_channel_id();
`,

	`-- Server-level bans table
CREATE TABLE IF NOT EXISTS server_bans (
    id BIGSERIAL PRIMARY KEY,
    server_id BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    banned_by BIGINT NOT NULL REFERENCES users(id),
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(server_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_server_bans_server_id ON server_bans(server_id);
CREATE INDEX IF NOT EXISTS idx_server_bans_user_id ON server_bans(user_id);
`,

	`-- Add bio field to users
ALTER TABLE users ADD COLUMN IF NOT EXISTS bio TEXT NOT NULL DEFAULT '';
`,

	`-- Add bot fields to users
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_bot BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS bot_owner_id BIGINT REFERENCES users(id) ON DELETE CASCADE;
`,

	`-- Add via_api to messages
ALTER TABLE messages ADD COLUMN IF NOT EXISTS via_api BOOLEAN NOT NULL DEFAULT FALSE;
`,

	`-- Create api_keys table
CREATE TABLE IF NOT EXISTS api_keys (
    id BIGSERIAL PRIMARY KEY,
    key_hash VARCHAR(64) NOT NULL UNIQUE,
    key_prefix VARCHAR(16) NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    owner_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_owner_id ON api_keys(owner_id);
`,

	`-- Add badges bitfield to users
ALTER TABLE users ADD COLUMN IF NOT EXISTS badges INTEGER NOT NULL DEFAULT 0;
`,

	`-- Add hoist and position to server_roles
ALTER TABLE server_roles ADD COLUMN IF NOT EXISTS hoist BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE server_roles ADD COLUMN IF NOT EXISTS position INTEGER NOT NULL DEFAULT 0;
`,

	`-- Add display_name to users
ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name VARCHAR(100) NOT NULL DEFAULT '';
`,

	`-- Add parent_id to messages for nested replies
ALTER TABLE messages ADD COLUMN IF NOT EXISTS parent_id BIGINT REFERENCES messages(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_messages_parent_id ON messages(parent_id) WHERE parent_id IS NOT NULL;

-- Create message_versions table for edit history
CREATE TABLE IF NOT EXISTS message_versions (
    id BIGSERIAL PRIMARY KEY,
    message_id BIGINT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    edited_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_message_versions_message_id ON message_versions(message_id, edited_at);
`,

	`-- Random 9-digit ID generator for bin posts
CREATE OR REPLACE FUNCTION gen_bin_post_id() RETURNS BIGINT LANGUAGE plpgsql VOLATILE AS $$
DECLARE new_id BIGINT;
BEGIN
  LOOP
    new_id := floor(random() * 900000000 + 100000000)::BIGINT;
    EXIT WHEN NOT EXISTS (SELECT 1 FROM bin_posts WHERE id = new_id);
  END LOOP;
  RETURN new_id;
END; $$;

-- Bin posts
CREATE TABLE IF NOT EXISTS bin_posts (
    id BIGINT PRIMARY KEY DEFAULT gen_bin_post_id(),
    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    thread_channel_id BIGINT REFERENCES channels(id) ON DELETE SET NULL,
    author_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(200) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    tags TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bin_posts_channel_id ON bin_posts(channel_id, created_at DESC);

-- Bin post files (current version)
CREATE TABLE IF NOT EXISTS bin_post_files (
    id BIGSERIAL PRIMARY KEY,
    post_id BIGINT NOT NULL REFERENCES bin_posts(id) ON DELETE CASCADE,
    filename VARCHAR(255) NOT NULL,
    language VARCHAR(50) NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    position INT NOT NULL DEFAULT 0,
    UNIQUE(post_id, position)
);
CREATE INDEX IF NOT EXISTS idx_bin_post_files_post_id ON bin_post_files(post_id, position);

-- Bin post versions (snapshots on edit)
CREATE TABLE IF NOT EXISTS bin_post_versions (
    id BIGSERIAL PRIMARY KEY,
    post_id BIGINT NOT NULL REFERENCES bin_posts(id) ON DELETE CASCADE,
    version INT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bin_post_versions_post_id ON bin_post_versions(post_id);

-- Bin post version files (file snapshots per version)
CREATE TABLE IF NOT EXISTS bin_post_version_files (
    id BIGSERIAL PRIMARY KEY,
    version_id BIGINT NOT NULL REFERENCES bin_post_versions(id) ON DELETE CASCADE,
    filename VARCHAR(255) NOT NULL,
    language VARCHAR(50) NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    position INT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_bin_post_version_files_version_id ON bin_post_version_files(version_id);

-- Line comments anchored to specific version/file/line
CREATE TABLE IF NOT EXISTS bin_line_comments (
    id BIGSERIAL PRIMARY KEY,
    post_id BIGINT NOT NULL REFERENCES bin_posts(id) ON DELETE CASCADE,
    version_id BIGINT NOT NULL REFERENCES bin_post_versions(id) ON DELETE CASCADE,
    file_id BIGINT NOT NULL REFERENCES bin_post_version_files(id) ON DELETE CASCADE,
    line_number INT NOT NULL,
    author_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    parent_id BIGINT REFERENCES bin_line_comments(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bin_line_comments_lookup ON bin_line_comments(version_id, file_id, line_number);

-- Admin-defined tags per bin channel
CREATE TABLE IF NOT EXISTS bin_channel_tags (
    id BIGSERIAL PRIMARY KEY,
    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    name VARCHAR(50) NOT NULL,
    color VARCHAR(7) NOT NULL DEFAULT '#99aab5',
    UNIQUE(channel_id, name)
);
CREATE INDEX IF NOT EXISTS idx_bin_channel_tags_channel_id ON bin_channel_tags(channel_id);
`,

	`-- Add password reset support
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_reset_token VARCHAR(64);
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_reset_expires_at TIMESTAMP;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_password_reset_token ON users(password_reset_token) WHERE password_reset_token IS NOT NULL;
`,

	`-- Add is_everyone flag to server_roles
ALTER TABLE server_roles ADD COLUMN IF NOT EXISTS is_everyone BOOLEAN NOT NULL DEFAULT FALSE;
CREATE UNIQUE INDEX IF NOT EXISTS idx_server_roles_everyone ON server_roles(server_id) WHERE is_everyone = TRUE;

-- Add synced flag to channels for category permission sync
ALTER TABLE channels ADD COLUMN IF NOT EXISTS synced BOOLEAN NOT NULL DEFAULT TRUE;

-- Create permission_overwrites table
CREATE TABLE IF NOT EXISTS permission_overwrites (
    id BIGSERIAL PRIMARY KEY,
    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    target_type SMALLINT NOT NULL,
    target_id BIGINT NOT NULL,
    allow BIGINT NOT NULL DEFAULT 0,
    deny BIGINT NOT NULL DEFAULT 0,
    UNIQUE(channel_id, target_type, target_id)
);
CREATE INDEX IF NOT EXISTS idx_perm_overwrites_channel ON permission_overwrites(channel_id);
CREATE INDEX IF NOT EXISTS idx_perm_overwrites_target ON permission_overwrites(target_type, target_id);

-- Remap existing permission bits to new layout
UPDATE server_roles SET permissions =
    (CASE WHEN permissions & 32 != 0 THEN 1 ELSE 0 END) |
    (CASE WHEN permissions & 16 != 0 THEN 2 ELSE 0 END) |
    (CASE WHEN permissions & 4  != 0 THEN 8 ELSE 0 END) |
    (CASE WHEN permissions & 8  != 0 THEN 16 ELSE 0 END) |
    (CASE WHEN permissions & 1  != 0 THEN 131072 ELSE 0 END) |
    (CASE WHEN permissions & 2  != 0 THEN 4194304 ELSE 0 END)
WHERE permissions != 0;

-- Rename conflicting roles
UPDATE server_roles SET name = 'everyone (renamed)' WHERE name = '@everyone';

-- Seed @everyone role for every server
INSERT INTO server_roles (server_id, name, color, permissions, hoist, position, is_everyone, created_at)
SELECT s.id, '@everyone', '#99aab5',
    (1::BIGINT << 16) | (1::BIGINT << 17) | (1::BIGINT << 23) | (1::BIGINT << 20) |
    (1::BIGINT << 18) | (1::BIGINT << 19) | (1::BIGINT << 32) | (1::BIGINT << 33) |
    (1::BIGINT << 37) | (1::BIGINT << 7) | (1::BIGINT << 8) | (1::BIGINT << 29),
    FALSE, 0, TRUE, NOW()
FROM servers s
WHERE NOT EXISTS (
    SELECT 1 FROM server_roles sr WHERE sr.server_id = s.id AND sr.is_everyone = TRUE
);
`,
	`-- Add passkeys table for WebAuthn/passkey authentication
CREATE TABLE IF NOT EXISTS passkeys (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id BYTEA NOT NULL UNIQUE,
    public_key BYTEA NOT NULL,
    sign_count BIGINT NOT NULL DEFAULT 0,
    aaguid BYTEA NOT NULL DEFAULT '\x',
    name VARCHAR(100) NOT NULL DEFAULT 'Passkey',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_passkeys_user_id ON passkeys(user_id);
`,

	`-- pg_trgm extension + GIN index for fast message content search
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX IF NOT EXISTS idx_messages_content_trgm ON messages USING GIN(content gin_trgm_ops);
`,

	`-- Track registration IP and last seen IP per user
ALTER TABLE users ADD COLUMN IF NOT EXISTS registration_ip VARCHAR(45);
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_seen_ip VARCHAR(45);
`,

	`-- Theming: user_themes and user_preferences tables
CREATE TABLE IF NOT EXISTS user_themes (
    id             SERIAL PRIMARY KEY,
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name           VARCHAR(64) NOT NULL,
    css            TEXT NOT NULL DEFAULT '',
    background_url VARCHAR(512),
    share_token    UUID UNIQUE,
    created_at     TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_themes_user_id ON user_themes(user_id);

CREATE TABLE IF NOT EXISTS user_preferences (
    user_id                BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    active_theme           VARCHAR(32) NOT NULL DEFAULT 'rory'
                             CHECK (active_theme IN (
                               'rory','citron-dark','citron-light',
                               'neon-nights','abyss','sakura','custom'
                             )),
    -- active_custom_theme_id uses INT (not BIGINT): user_themes.id is SERIAL (4-byte)
    active_custom_theme_id INT REFERENCES user_themes(id) ON DELETE SET NULL
);
`,

	`-- Add base_theme column to user_themes
ALTER TABLE user_themes ADD COLUMN IF NOT EXISTS base_theme VARCHAR(32) NOT NULL DEFAULT 'rory';
`,

	`-- Add is_published and is_featured columns for the public theme repository
ALTER TABLE user_themes ADD COLUMN IF NOT EXISTS is_published BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE user_themes ADD COLUMN IF NOT EXISTS is_featured BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_user_themes_published ON user_themes(is_published) WHERE is_published = TRUE;
`,

	`-- Add source_share_token to track which published theme a user installed (non-unique)
ALTER TABLE user_themes ADD COLUMN IF NOT EXISTS source_share_token UUID;
CREATE INDEX IF NOT EXISTS idx_user_themes_source_token ON user_themes(user_id, source_share_token) WHERE source_share_token IS NOT NULL;
`,

	`-- Track cumulative upload bytes per user for storage quota enforcement
ALTER TABLE users ADD COLUMN IF NOT EXISTS upload_bytes_used BIGINT NOT NULL DEFAULT 0;
`,

	`-- Track individual uploads per user for rolling eviction when quota is hit
CREATE TABLE IF NOT EXISTS user_uploads (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    spaces_key VARCHAR(512) NOT NULL,
    file_size  BIGINT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_uploads_user_age ON user_uploads(user_id, created_at ASC);
`,

	`-- Migration 31: Bots & AI Chatbot
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_verified BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS server_bots (
  server_id   BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  bot_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  added_at    TIMESTAMP NOT NULL DEFAULT NOW(),
  PRIMARY KEY (server_id, bot_user_id)
);
CREATE INDEX IF NOT EXISTS idx_server_bots_bot ON server_bots(bot_user_id);

CREATE TABLE IF NOT EXISTS server_ai_config (
  server_id     BIGINT PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
  provider      VARCHAR(32)  NOT NULL DEFAULT 'parley',
  model         VARCHAR(128) NOT NULL DEFAULT 'ministral-3:14b',
  api_key_enc   TEXT,
  system_prompt TEXT NOT NULL DEFAULT '',
  updated_at    TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS server_bot_usage (
  server_id   BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  month       DATE   NOT NULL,
  tokens_used BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (server_id, month)
);

CREATE TABLE IF NOT EXISTS bot_invite_tokens (
  id          BIGSERIAL PRIMARY KEY,
  bot_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token       UUID   NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at  TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bot_invite_tokens_bot ON bot_invite_tokens(bot_user_id);

-- Seed AI Chatbot system user (email nullable since migration 11)
INSERT INTO users (username, display_name, password_hash, is_bot, is_verified)
VALUES ('ai-chatbot', 'AI Chatbot', '', TRUE, TRUE)
ON CONFLICT (username) DO NOTHING;

-- Seed permanent invite token (fixed UUID so it can be referenced in config/docs)
INSERT INTO bot_invite_tokens (bot_user_id, token)
SELECT id, 'aaaaaaaa-0000-0000-0000-000000000001'::uuid
FROM users WHERE username = 'ai-chatbot'
ON CONFLICT (token) DO NOTHING;
`,
	`-- Migration 32: add created_by to bot_invite_tokens for "Your Bots" ownership tracking
ALTER TABLE bot_invite_tokens ADD COLUMN IF NOT EXISTS created_by BIGINT REFERENCES users(id);
`,
	`-- Migration 33: rename ai-chatbot to Polly; add per-server bot degraded state
UPDATE users SET username='polly', display_name='Polly' WHERE username='ai-chatbot' AND is_bot=TRUE;

ALTER TABLE server_bots ADD COLUMN IF NOT EXISTS last_error_at TIMESTAMPTZ;
ALTER TABLE server_bots ADD COLUMN IF NOT EXISTS is_degraded BOOLEAN NOT NULL DEFAULT FALSE;
`,
	`-- Add reply (parent_id) and reactions to DM messages
ALTER TABLE dm_messages ADD COLUMN IF NOT EXISTS parent_id BIGINT REFERENCES dm_messages(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_dm_messages_parent_id ON dm_messages(parent_id) WHERE parent_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS dm_message_reactions (
    id BIGSERIAL PRIMARY KEY,
    message_id BIGINT NOT NULL REFERENCES dm_messages(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji VARCHAR(64) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(message_id, user_id, emoji)
);
CREATE INDEX IF NOT EXISTS idx_dm_message_reactions_message_id ON dm_message_reactions(message_id);
`,
	`-- Migration 35: ensure DM parity schema is applied (idempotent re-run guards against bootstrap skipping migration 34)
ALTER TABLE dm_messages ADD COLUMN IF NOT EXISTS parent_id BIGINT REFERENCES dm_messages(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_dm_messages_parent_id ON dm_messages(parent_id) WHERE parent_id IS NOT NULL;
CREATE TABLE IF NOT EXISTS dm_message_reactions (
    id BIGSERIAL PRIMARY KEY,
    message_id BIGINT NOT NULL REFERENCES dm_messages(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji VARCHAR(64) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(message_id, user_id, emoji)
);
CREATE INDEX IF NOT EXISTS idx_dm_message_reactions_message_id ON dm_message_reactions(message_id);
`,

	`-- Migration 36: friend requests
CREATE TABLE IF NOT EXISTS friend_requests (
    id          BIGSERIAL PRIMARY KEY,
    sender_id   BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    receiver_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status      VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(sender_id, receiver_id)
);
CREATE INDEX IF NOT EXISTS idx_friend_requests_receiver ON friend_requests(receiver_id);
CREATE INDEX IF NOT EXISTS idx_friend_requests_sender   ON friend_requests(sender_id);
`,
}

// MigrationSQL returns all migrations as a single concatenated string
func MigrationSQL() string {
	result := ""
	for _, m := range Migrations {
		result += m + "\n"
	}
	return result
}