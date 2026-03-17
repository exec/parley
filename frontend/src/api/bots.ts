// frontend/src/api/bots.ts
import { apiClient } from './client';

export interface BotSummary {
  id: number;
  username: string;
  display_name: string;
  avatar_url?: string;
  is_verified: boolean;
  added_at: string;
}

export interface AIConfig {
  provider: string;
  model: string;
  api_key_set: boolean;
  preset_verbosity: string;
  preset_personality: string;
  preset_role: string;
  updated_at: string;
}

export interface AIUsage {
  tokens_used: number;
  tokens_limit: number;
  model: string;
  resets_at: string;
}

export interface BotInviteInfo {
  bot_id: number;
  username: string;
  display_name: string;
  avatar_url?: string;
  is_verified: boolean;
}

export const listBots = (serverId: number) =>
  apiClient.get<BotSummary[]>(`/servers/${serverId}/bots`);

export const addBot = (serverId: number, inviteToken: string) =>
  apiClient.post<void>(`/servers/${serverId}/bots`, { invite_token: inviteToken });

export const removeBot = (serverId: number, botId: number) =>
  apiClient.delete<void>(`/servers/${serverId}/bots/${botId}`);

export const getAIConfig = (serverId: number) =>
  apiClient.get<AIConfig>(`/servers/${serverId}/ai-config`);

export const setAIConfig = (serverId: number, data: {
  provider: string; model: string; api_key?: string;
  preset_verbosity: string; preset_personality: string; preset_role: string;
}) => apiClient.put<void>(`/servers/${serverId}/ai-config`, data);

export const getAIUsage = (serverId: number) =>
  apiClient.get<AIUsage>(`/servers/${serverId}/ai-usage`);

export interface UserBot {
  id: number;
  username: string;
  display_name: string;
  avatar_url?: string;
  is_verified: boolean;
  invite_token: string;
}

export const getMyBots = () =>
  apiClient.get<UserBot[]>('/bots/mine');

export const resolveBotInvite = (token: string) =>
  apiClient.get<BotInviteInfo>(`/bots/invite/${token}`);

export const acceptBotInvite = (token: string, serverId: number) =>
  apiClient.post<void>(`/bots/invite/${token}/accept`, { server_id: serverId });

export const PROVIDER_MODELS: Record<string, { label: string; value: string }[]> = {
  parley: [
    { label: 'Ministral 3 (14B)', value: 'ministral-3:14b' },
    { label: 'GPT-OSS (20B)',      value: 'gpt-oss:20b' },
    { label: 'Gemma 3 (27B)',      value: 'gemma3:27b' },
    { label: 'GPT-OSS (120B)',     value: 'gpt-oss:120b' },
    { label: 'Qwen3.5 (397B)',     value: 'qwen3:latest' },
  ],
  anthropic: [
    { label: 'Claude Opus 4.5',         value: 'claude-opus-4-5' },
    { label: 'Claude Sonnet 4.5',       value: 'claude-sonnet-4-5' },
    { label: 'Claude Haiku 4.5',        value: 'claude-haiku-4-5-20251001' },
  ],
  openai: [
    { label: 'GPT-4.1',      value: 'gpt-4.1' },
    { label: 'GPT-4.1 Mini', value: 'gpt-4.1-mini' },
    { label: 'GPT-4o',       value: 'gpt-4o' },
    { label: 'o3-mini',      value: 'o3-mini' },
  ],
  xai: [
    { label: 'Grok 3',      value: 'grok-3' },
    { label: 'Grok 3 Mini', value: 'grok-3-mini' },
  ],
  mistral: [
    { label: 'Mistral Large',  value: 'mistral-large-latest' },
    { label: 'Mistral Small',  value: 'mistral-small-latest' },
    { label: 'Codestral',      value: 'codestral-latest' },
  ],
  google: [
    { label: 'Gemini 2.5 Pro',       value: 'gemini-2.5-pro' },
    { label: 'Gemini 2.0 Flash',     value: 'gemini-2.0-flash' },
    { label: 'Gemini 2.0 Flash Lite',value: 'gemini-2.0-flash-lite' },
  ],
};

export const PROVIDER_LABELS: Record<string, string> = {
  parley: 'Parley', anthropic: 'Anthropic', openai: 'OpenAI',
  xai: 'xAI', mistral: 'Mistral', google: 'Google',
};

// Permanent invite tokens seeded by migration — never expire.
export const OFFICIAL_BOTS: { username: string; displayName: string; description: string; token: string }[] = [
  {
    username: 'polly',
    displayName: 'Polly',
    description: 'Responds to @mentions with AI. Supports Parley, Anthropic, OpenAI, and more.',
    token: 'aaaaaaaa-0000-0000-0000-000000000001',
  },
];

// Monthly compute-credit budget (same for all servers; usage is scaled by model cost factor on the server)
export const PARLEY_MONTHLY_BUDGET = 2_000_000;
