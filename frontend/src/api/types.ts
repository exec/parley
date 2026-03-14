export interface User {
  id: string;
  username: string;
  email: string;
}

export interface Server {
  id: string;
  name: string;
  icon_url?: string;
  owner_id: string;
  created_at: string;
  updated_at: string;
}

export interface ServerMember {
  id: string;
  server_id: string;
  user_id: string;
  username: string;
  nickname?: string;
  joined_at: string;
}

export interface Channel {
  id: string;
  server_id: string;
  name: string;
  type: number;
  created_at: string;
  updated_at: string;
}

export interface Message {
  id: string;
  channel_id: string;
  author_id: string;
  author_username: string;
  content: string;
  created_at: string;
  updated_at: string;
}

export interface AuthResponse {
  user: User;
  token: string;
}

export interface ApiError {
  message: string;
  code?: string;
}
