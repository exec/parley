export interface AppNotification {
  id: string;
  type: 'mention' | 'dm' | 'friend_request' | 'friend_accept';
  title: string;
  body: string;
  actor_username: string;
  actor_avatar_url?: string;
  server_id?: string;
  channel_id?: string;
  message_id?: string;
  dm_channel_id?: string;
  read: boolean;
  created_at: string;
}

export interface User {
  id: string;
  username: string;
  display_name?: string;
  email: string;
  avatar_url?: string;
  banner_url?: string;
  bio?: string;
  badges?: number;
  email_verified?: boolean;
  phone_number?: string;
  phone_verified?: boolean;
  has_password?: boolean;
  status_type?: 'online' | 'dnd' | 'afk' | 'invisible';
  status_text?: string;
}

export interface Role {
  id: string;
  server_id: string;
  name: string;
  color: string;
  permissions: number;
  hoist: boolean;
  position: number;
  is_everyone?: boolean;
  created_at: string;
}

export interface Server {
  id: string;
  name: string;
  icon_url?: string;
  owner_id: string;
  vanity_url?: string;
  description?: string;
  is_public?: boolean;
  created_at: string;
  updated_at: string;
}

export interface ServerCategory {
  id: number;
  name: string;
  created_at?: string;
}

export interface PublicServer {
  id: string;
  name: string;
  icon_url?: string;
  vanity_url: string;
  description?: string;
  member_count: number;
  categories: ServerCategory[];
}

export interface ServerMember {
  id: string;
  server_id: string;
  user_id: string;
  username: string;
  display_name?: string;
  nickname?: string;
  avatar_url?: string;
  banner_url?: string;
  bio?: string;
  badges?: number;
  joined_at: string;
  roles?: Role[];
  is_bot?: boolean;
  bot_degraded?: boolean;
  invite_code?: string;
  status_type?: 'online' | 'dnd' | 'afk' | 'invisible';
  status_text?: string;
}

export interface Channel {
  id: string;
  server_id: string;
  name: string;
  type: number;
  position: number;
  parent_id?: string;
  topic?: string;
  created_at: string;
  updated_at: string;
}

export interface Reaction {
  emoji: string;
  count: number;
  user_ids: string[];
}

export interface ForwardedMessage {
  message_id: string;
  channel_id?: string;
  channel_name?: string;
  server_id?: string;
  server_name?: string;
  author_username: string;
  author_display_name?: string;
  author_avatar_url?: string;
  content?: string;
  attachment_name?: string;
  created_at: string;
}

export interface Message {
  id: string;
  channel_id: string;
  author_id: string;
  author_username: string;
  author_display_name?: string;
  author_avatar_url?: string;
  author_is_bot?: boolean;
  via_api?: boolean;
  content: string;
  nonce?: string;
  created_at: string;
  updated_at: string;
  reactions?: Reaction[];
  pending?: boolean; // optimistic: true until confirmed by WS event
  attachment_url?: string;
  attachment_name?: string;
  attachment_type?: string;
  parent_id?: string;
  parent_author_username?: string;
  parent_author_display_name?: string;
  is_pinned?: boolean;
  pinned_at?: string;
  forwarded_message?: ForwardedMessage;
}

export interface AuthResponse {
  user: User;
  token: string;
}

export interface ApiError {
  message: string;
  code?: string;
}

export interface DmChannel {
  id: string;
  user1_id: string;
  user2_id: string;
  created_at: string;
  other_username: string;
  other_display_name?: string;
  other_user_id: string;
  other_avatar_url?: string;
}

export interface DmMessage {
  id: string;
  dm_channel_id: string;
  author_id: string;
  author_username: string;
  author_display_name?: string;
  author_avatar_url?: string;
  content: string;
  parent_id?: string;
  parent_author_username?: string;
  parent_author_display_name?: string;
  reactions?: Reaction[];
  created_at: string;
  updated_at: string;
  attachment_url?: string;
  attachment_name?: string;
  attachment_type?: string;
  forwarded_message?: ForwardedMessage;
}

export interface PublicUser {
  id: string;
  username: string;
  display_name?: string;
  avatar_url: string;
  created_at: string;
  banner_url?: string;
  bio?: string;
  badges?: number;
  status_type?: 'online' | 'dnd' | 'afk' | 'invisible';
  status_text?: string;
}

export interface BinPost {
  id: string; channel_id: string; thread_channel_id: string;
  author_id: string; title: string; description: string;
  tags: string[]; created_at: string; updated_at: string;
  author_username: string; author_avatar_url?: string;
  files: BinPostFile[]; comment_count: number;
  line_comment_count: number; version_count: number;
}

export interface BinPostFile {
  id: string; post_id: string; filename: string;
  language: string; content: string; position: number;
}

export interface BinPostVersion {
  id: string; post_id: string; version: number;
  description: string; created_at: string;
  files?: BinPostVersionFile[];
}

export interface BinPostVersionFile {
  id: string; version_id: string; filename: string;
  language: string; content: string; position: number;
}

export interface BinLineComment {
  id: string; post_id: string; version_id: string;
  file_id: string; line_number: number; author_id: string;
  content: string; parent_id?: string;
  created_at: string; updated_at: string;
  author_username: string; author_avatar_url?: string;
}

export interface BinChannelTag {
  id: string; channel_id: string; name: string; color: string;
}

export interface FriendUser {
  id: string;
  username: string;
  display_name?: string;
  avatar_url?: string;
}

export interface FriendRequest {
  id: string;
  sender_id: string;
  receiver_id: string;
  status: 'pending' | 'accepted';
  user: FriendUser; // always the other party
  created_at: string;
}

export interface FriendRequestsResponse {
  incoming: FriendRequest[];
  outgoing: FriendRequest[];
}

// ----- Slash commands (Phase 1) -----

export type SlashOptionType = 'STRING' | 'INTEGER' | 'BOOLEAN';

export interface SlashOptionChoice {
  name: string;
  value: string | number;
}

export interface SlashCommandOption {
  name: string;
  description: string;
  type: SlashOptionType;
  required?: boolean;
  choices?: SlashOptionChoice[];
  min_value?: number;
  max_value?: number;
  min_length?: number;
  max_length?: number;
}

export interface BotCommand {
  id: number;
  bot_id: number;
  server_id: number;
  name: string;
  description: string;
  options: SlashCommandOption[];
  bot_username?: string;
  bot_display_name?: string;
  bot_avatar_url?: string;
}

export interface InteractionInvokeResponse {
  interaction_id: string;
  status: 'pending' | 'responded' | 'expired';
  expires_at: string;
}

// Cross-cutting per-(user, channel) read-state and notification settings.
// Applies to both server channels (kind=1) and DM channels (kind=2).
export type NotificationSetting = 'ALL' | 'MENTIONS_ONLY' | 'MUTED';
export const NOTIFICATION_SETTINGS: NotificationSetting[] = ['ALL', 'MENTIONS_ONLY', 'MUTED'];

export type ChannelKind = 1 | 2; // 1=server, 2=dm
export const CHANNEL_KIND_SERVER: ChannelKind = 1;
export const CHANNEL_KIND_DM: ChannelKind = 2;

export interface UserChannelState {
  user_id: string;
  channel_kind: ChannelKind;
  channel_id: string;
  last_read_message_id: string | null;
  notification_setting: 0 | 1 | 2;
  updated_at: string;
}
