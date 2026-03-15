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
}

export interface Role {
  id: string;
  server_id: string;
  name: string;
  color: string;
  permissions: number;
  hoist: boolean;
  position: number;
  created_at: string;
}

export interface Server {
  id: string;
  name: string;
  icon_url?: string;
  owner_id: string;
  vanity_url?: string;
  created_at: string;
  updated_at: string;
}

export interface ServerMember {
  id: string;
  server_id: string;
  user_id: string;
  username: string;
  nickname?: string;
  avatar_url?: string;
  banner_url?: string;
  bio?: string;
  badges?: number;
  joined_at: string;
  roles?: Role[];
}

export interface Channel {
  id: string;
  server_id: string;
  name: string;
  type: number;
  topic?: string;
  created_at: string;
  updated_at: string;
}

export interface Reaction {
  emoji: string;
  count: number;
  user_ids: string[];
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
  other_user_id: string;
  other_avatar_url?: string;
}

export interface DmMessage {
  id: string;
  dm_channel_id: string;
  author_id: string;
  author_username: string;
  author_avatar_url?: string;
  content: string;
  created_at: string;
  updated_at: string;
  attachment_url?: string;
  attachment_name?: string;
  attachment_type?: string;
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
}
