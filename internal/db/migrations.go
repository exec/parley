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
    id BIGSERIAL PRIMARY KEY DEFAULT gen_bin_post_id(),
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
}

// MigrationSQL returns all migrations as a single concatenated string
func MigrationSQL() string {
	result := ""
	for _, m := range Migrations {
		result += m + "\n"
	}
	return result
}