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

	`-- Migration 37: Grant PermCreatePosts (bit 1<<29) to all existing @everyone roles.
-- Servers created before the Bin feature was added have this bit missing from their
-- @everyone permissions, which prevents all members from creating posts.
UPDATE server_roles
SET permissions = permissions | (1::bigint << 29)
WHERE name = '@everyone'
  AND (permissions & (1::bigint << 29)) = 0;
`,

	`ALTER TABLE invites
    ADD COLUMN IF NOT EXISTS max_uses INT DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMP DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS use_count INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMP DEFAULT NULL;
ALTER TABLE server_members
    ADD COLUMN IF NOT EXISTS invite_code VARCHAR(16) DEFAULT NULL;
CREATE INDEX IF NOT EXISTS idx_server_members_invite_code ON server_members(invite_code) WHERE invite_code IS NOT NULL;`,

	`-- Store WebAuthn credential flags required for login validation
ALTER TABLE passkeys
    ADD COLUMN IF NOT EXISTS backup_eligible BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS backup_state    BOOLEAN NOT NULL DEFAULT false;`,

	`-- Add user status columns
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS status_type VARCHAR(16) NOT NULL DEFAULT 'online',
  ADD COLUMN IF NOT EXISTS status_text VARCHAR(128) NOT NULL DEFAULT '';`,

	`ALTER TABLE bot_invite_tokens
  ADD COLUMN IF NOT EXISTS permissions BIGINT NOT NULL DEFAULT 0;`,

	`-- Backfill created_by on bot_invite_tokens from bot_owner_id on users,
-- and create missing invite token rows for bots that never had one.
UPDATE bot_invite_tokens bit
SET created_by = u.bot_owner_id
FROM users u
WHERE u.id = bit.bot_user_id
  AND u.bot_owner_id IS NOT NULL
  AND bit.created_by IS NULL;

INSERT INTO bot_invite_tokens (bot_user_id, token, created_by)
SELECT u.id, gen_random_uuid(), u.bot_owner_id
FROM users u
WHERE u.is_bot = TRUE
  AND u.bot_owner_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM bot_invite_tokens bit WHERE bit.bot_user_id = u.id);`,

	`ALTER TABLE bot_invite_tokens ADD CONSTRAINT bot_invite_tokens_bot_user_id_key UNIQUE (bot_user_id);`,

	`ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verification_token_expires_at TIMESTAMPTZ;
UPDATE users SET email_verification_token_expires_at = created_at + INTERVAL '72 hours' WHERE email_verification_token IS NOT NULL AND email_verification_token_expires_at IS NULL;`,

	`ALTER TABLE bot_invite_tokens ADD COLUMN IF NOT EXISTS show_author BOOLEAN NOT NULL DEFAULT FALSE;`,

	`-- Soundboard sounds
	CREATE TABLE IF NOT EXISTS soundboard_sounds (
	    id          BIGSERIAL PRIMARY KEY,
	    server_id   BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
	    uploader_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
	    name        VARCHAR(32) NOT NULL,
	    emoji       VARCHAR(64),
	    file_url    TEXT NOT NULL,
	    file_key    TEXT NOT NULL,
	    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_soundboard_sounds_server ON soundboard_sounds(server_id);`,

	`-- Pinned messages
	CREATE TABLE IF NOT EXISTS pinned_messages (
	    channel_id  BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
	    message_id  BIGINT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
	    pinned_by   BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	    pinned_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	    PRIMARY KEY (channel_id, message_id)
	);
	CREATE INDEX IF NOT EXISTS idx_pinned_messages_channel ON pinned_messages(channel_id);`,

	`-- Forwarded message data (JSONB snapshot stored at forward time)
	ALTER TABLE messages ADD COLUMN IF NOT EXISTS forwarded_data JSONB;
	ALTER TABLE dm_messages ADD COLUMN IF NOT EXISTS forwarded_data JSONB;`,

	`-- In-app notification center
	CREATE TABLE IF NOT EXISTS notifications (
		id BIGSERIAL PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		type VARCHAR(50) NOT NULL,
		title TEXT NOT NULL DEFAULT '',
		body TEXT NOT NULL DEFAULT '',
		actor_username TEXT NOT NULL DEFAULT '',
		actor_avatar_url TEXT NOT NULL DEFAULT '',
		server_id BIGINT REFERENCES servers(id) ON DELETE CASCADE,
		channel_id BIGINT REFERENCES channels(id) ON DELETE SET NULL,
		message_id BIGINT REFERENCES messages(id) ON DELETE SET NULL,
		dm_channel_id BIGINT REFERENCES dm_channels(id) ON DELETE CASCADE,
		read BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_notifications_user_id ON notifications(user_id, created_at DESC);`,

	`-- Server discovery: description + is_public on servers; server_categories; server_category_assignments
	ALTER TABLE servers ADD COLUMN IF NOT EXISTS description TEXT;
	ALTER TABLE servers ADD COLUMN IF NOT EXISTS is_public BOOLEAN NOT NULL DEFAULT FALSE;
	CREATE INDEX IF NOT EXISTS idx_servers_is_public ON servers(is_public) WHERE is_public = TRUE;

	CREATE TABLE IF NOT EXISTS server_categories (
	    id         BIGSERIAL PRIMARY KEY,
	    name       VARCHAR(100) NOT NULL UNIQUE,
	    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS server_category_assignments (
	    server_id   BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
	    category_id BIGINT NOT NULL REFERENCES server_categories(id) ON DELETE CASCADE,
	    PRIMARY KEY (server_id, category_id)
	);
	CREATE INDEX IF NOT EXISTS idx_sca_category_id ON server_category_assignments(category_id);`,

	`-- Widen invites.code to accommodate 6-byte (12-char) hex codes from S3 entropy fix
ALTER TABLE invites ALTER COLUMN code TYPE VARCHAR(16);`,

	`-- Server audit log
CREATE TABLE IF NOT EXISTS server_audit_logs (
    id              BIGSERIAL PRIMARY KEY,
    server_id       BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    actor_id        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    actor_username  TEXT NOT NULL DEFAULT '',
    action          VARCHAR(50) NOT NULL,
    target_id       TEXT,
    target_type     VARCHAR(20),
    target_name     TEXT,
    changes         JSONB,
    reason          TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sal_server_time ON server_audit_logs(server_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sal_actor       ON server_audit_logs(server_id, actor_id) WHERE actor_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sal_action      ON server_audit_logs(server_id, action);`,

	`-- Fix duplicate positions before adding unique constraint: assign sequential
-- positions within each server based on creation order.
WITH numbered AS (
    SELECT id, server_id,
           ROW_NUMBER() OVER (PARTITION BY server_id ORDER BY created_at, id) - 1 AS new_pos
    FROM server_roles
)
UPDATE server_roles SET position = numbered.new_pos
FROM numbered WHERE server_roles.id = numbered.id AND server_roles.position != numbered.new_pos;

-- Add unique constraint on server role positions to prevent duplicate positions during concurrent reorders
CREATE UNIQUE INDEX IF NOT EXISTS idx_server_roles_unique_position ON server_roles(server_id, position);`,

	`-- Slash commands Phase 1: bot_commands table stores command definitions
-- registered by a bot for a particular server. (bot_id, server_id, name) is
-- unique so re-registering the same name is an upsert.
CREATE TABLE IF NOT EXISTS bot_commands (
    id          BIGSERIAL PRIMARY KEY,
    bot_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    server_id   BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name        VARCHAR(32) NOT NULL,
    description VARCHAR(100) NOT NULL,
    options     JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (bot_id, server_id, name)
);
CREATE INDEX IF NOT EXISTS idx_bot_commands_server ON bot_commands(server_id);
CREATE INDEX IF NOT EXISTS idx_bot_commands_bot ON bot_commands(bot_id);`,

	`-- Slash commands Phase 1: bot_interactions table stores one invocation of a
-- slash command. token is a 64-char random string used as a bearer credential
-- for POST /api/interactions/{token}/respond. Rows live at most 15 minutes.
CREATE TABLE IF NOT EXISTS bot_interactions (
    token               VARCHAR(64) PRIMARY KEY,
    bot_id              BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    command_id          BIGINT NOT NULL REFERENCES bot_commands(id) ON DELETE CASCADE,
    invoker_user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id          BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    server_id           BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    options             JSONB NOT NULL DEFAULT '{}'::jsonb,
    state               VARCHAR(16) NOT NULL DEFAULT 'pending',
    response_message_id BIGINT REFERENCES messages(id) ON DELETE SET NULL,
    expires_at          TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bot_interactions_expires ON bot_interactions(expires_at);
CREATE INDEX IF NOT EXISTS idx_bot_interactions_bot ON bot_interactions(bot_id);`,

	`-- Slash commands Phase 1: messages.kind tags a message's origin. Values used
-- this phase: 'normal', 'interaction_response', 'system'. No CHECK constraint
-- so new kinds can be added by later migrations without a column rewrite.
ALTER TABLE messages ADD COLUMN IF NOT EXISTS kind VARCHAR(16) NOT NULL DEFAULT 'normal';`,

	`-- Invite-only registration. invite_count tracks how many codes a user may
-- still generate; registration_invites stores each generated code with the
-- inviter, and (when consumed) the invitee and used_at. Every existing
-- non-system, non-bot user starts with 1 invite.
ALTER TABLE users ADD COLUMN IF NOT EXISTS invite_count INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS registration_invites (
    code        VARCHAR(16) PRIMARY KEY,
    inviter_id  BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    invitee_id  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    used_at     TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_registration_invites_inviter ON registration_invites(inviter_id);

UPDATE users SET invite_count = 1 WHERE is_system = FALSE AND is_bot = FALSE AND invite_count = 0;`,

	// D3 (audit 2026-04-23): bot API keys now carry a scope array. Existing
	// rows grandfather to {'full'} so deployed bots keep working while their
	// owners rotate down to narrower scopes; new rows default to '{}' which
	// CreateAPIKey / CreateBotWithKey overwrite with whatever the caller
	// passed. An empty array at lookup time means a pre-migration key still
	// in flight during a rolling deploy — the middleware treats that as
	// no-scopes (HasScope always false), which is the safe failure mode.
	`ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS scopes TEXT[] NOT NULL DEFAULT '{}';
UPDATE api_keys SET scopes = ARRAY['full']::TEXT[] WHERE scopes = '{}'::TEXT[];`,

	// Migration #64: cross-cutting per-(user, channel) read-state and notification
	// settings. Used by both server channels (channel_kind = 1) and DM channels
	// (channel_kind = 2). Rows are written only when a user marks-read or changes
	// notification setting; default state (no row) = NotificationAll, never-read.
	`CREATE TABLE IF NOT EXISTS user_channel_state (
    user_id              BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_kind         SMALLINT    NOT NULL CHECK (channel_kind IN (1, 2)),
    channel_id           BIGINT      NOT NULL,
    last_read_message_id BIGINT,
    notification_setting SMALLINT    NOT NULL DEFAULT 0,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, channel_kind, channel_id)
);
CREATE INDEX IF NOT EXISTS user_channel_state_user_idx ON user_channel_state(user_id);`,

	// Migration #65: generalize dm_channels for group DM support. Adds is_group
	// (false = legacy 1:1, true = group), optional name/avatar_url, and
	// created_by/owner_user_id for group ownership. Introduces dm_channel_members
	// as the canonical membership table so server code can query members
	// uniformly regardless of group-vs-1:1 — the backfill seeds it from the
	// existing user1_id/user2_id columns (those columns stay for now to keep
	// the migration safe; later cleanup can drop them once all readers move to
	// dm_channel_members). Also adds dm_messages.system_event JSONB to carry
	// structured payloads for member-added / name-changed / etc. system
	// messages in group DMs (NULL for normal messages).
	`ALTER TABLE dm_channels
    ADD COLUMN IF NOT EXISTS is_group           BOOLEAN     NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS name               TEXT,
    ADD COLUMN IF NOT EXISTS avatar_url         TEXT,
    ADD COLUMN IF NOT EXISTS created_by_user_id BIGINT      REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS owner_user_id      BIGINT      REFERENCES users(id);

CREATE TABLE IF NOT EXISTS dm_channel_members (
    dm_channel_id BIGINT      NOT NULL REFERENCES dm_channels(id) ON DELETE CASCADE,
    user_id       BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (dm_channel_id, user_id)
);
CREATE INDEX IF NOT EXISTS dm_channel_members_user_idx ON dm_channel_members(user_id);

INSERT INTO dm_channel_members (dm_channel_id, user_id, joined_at)
SELECT id, user1_id, created_at FROM dm_channels
UNION ALL
SELECT id, user2_id, created_at FROM dm_channels
ON CONFLICT DO NOTHING;

ALTER TABLE dm_messages ADD COLUMN IF NOT EXISTS system_event JSONB;`,

	// Migration #66: drop NOT NULL on dm_channels.user1_id/user2_id so group
	// channels (where there's no canonical "two parties") can leave them
	// NULL rather than carry placeholder values. The 1:1 paths still set
	// both columns; the unique (user1_id, user2_id) constraint continues to
	// prevent duplicate 1:1 pairs because Postgres treats NULLs as distinct
	// in unique indexes — multiple (NULL, NULL) rows for groups are fine.
	`ALTER TABLE dm_channels ALTER COLUMN user1_id DROP NOT NULL;
ALTER TABLE dm_channels ALTER COLUMN user2_id DROP NOT NULL;`,
}

// MigrationSQL returns all migrations as a single concatenated string
func MigrationSQL() string {
	result := ""
	for _, m := range Migrations {
		result += m + "\n"
	}
	return result
}