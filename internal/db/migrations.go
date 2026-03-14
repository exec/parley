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
}

// MigrationSQL returns all migrations as a single concatenated string
func MigrationSQL() string {
	result := ""
	for _, m := range Migrations {
		result += m + "\n"
	}
	return result
}