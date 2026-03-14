// ============================================================
// PARLEY ADMIN — API Client
// ============================================================

const BASE = '/api'

export function getToken(): string | null {
  return localStorage.getItem('admin_token')
}

function authHeaders(): Record<string, string> {
  const token = getToken()
  if (!token) return {}
  return { Authorization: 'Bearer ' + token }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  params?: Record<string, string | number | undefined>
): Promise<T> {
  let url = BASE + path
  if (params) {
    const qs = Object.entries(params)
      .filter(([, v]) => v !== undefined && v !== '')
      .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`)
      .join('&')
    if (qs) url += '?' + qs
  }
  const res = await fetch(url, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...authHeaders(),
    },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    let msg = `HTTP ${res.status}`
    try {
      const data = await res.json()
      msg = data.error || data.message || msg
    } catch {
      // ignore
    }
    throw new Error(msg)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

// ---- Auth ----

export interface LoginResponse {
  token: string
  username: string
}

export async function apiLogin(username: string, password: string): Promise<LoginResponse> {
  return request<LoginResponse>('POST', '/auth/login', { username, password })
}

// ---- Stats ----

export interface Stats {
  total_users: number
  new_users_today: number
  total_messages: number
  total_servers: number
  open_reports: number
  banned_users: number
}

export async function apiGetStats(): Promise<Stats> {
  return request<Stats>('GET', '/stats')
}

// ---- Users ----
// Go returns db.User with int64 ids serialized as JSON numbers

export interface User {
  id: number
  username: string
  email: string
  phone_number?: string
  phone_verified: boolean
  email_verified: boolean
  avatar_url?: string
  banned_at?: string       // present if banned
  ban_reason?: string
  force_logout_at?: string
  is_system: boolean
  created_at: string
  updated_at: string
}

// Derived helper — no "status" field on User, derive from banned_at
export function userStatus(u: User): 'active' | 'banned' {
  return u.banned_at ? 'banned' : 'active'
}

// GET /api/users returns []User directly
export async function apiGetUsers(q?: string, limit = 50, offset = 0): Promise<User[]> {
  return request<User[]>('GET', '/users', undefined, { q, limit, offset })
}

export async function apiGetUser(id: number): Promise<User> {
  return request<User>('GET', `/users/${id}`)
}

export async function apiBanUser(id: number, reason: string): Promise<void> {
  return request<void>('POST', `/users/${id}/ban`, { reason })
}

export async function apiUnbanUser(id: number): Promise<void> {
  return request<void>('POST', `/users/${id}/unban`)
}

export async function apiForceLogout(id: number): Promise<void> {
  return request<void>('POST', `/users/${id}/force-logout`)
}

export async function apiImpersonateUser(id: number): Promise<{ token: string }> {
  return request<{ token: string }>('POST', `/users/${id}/impersonate`)
}

export async function apiDeleteUser(id: number): Promise<void> {
  return request<void>('DELETE', `/users/${id}`)
}

// ---- Messages ----
// Go returns db.Message; channel_id and author_id are int64 → numbers

export interface Message {
  id: number
  channel_id: number
  author_id: number
  author_username: string
  content: string
  nonce?: string
  attachment_url?: string
  attachment_name?: string
  attachment_type?: string
  created_at: string
  updated_at: string
}

// GET /api/messages returns []Message directly
export async function apiGetMessages(
  q?: string,
  user_id?: number,
  limit = 50,
  offset = 0
): Promise<Message[]> {
  return request<Message[]>('GET', '/messages', undefined, {
    q,
    user_id: user_id || undefined,
    limit,
    offset,
  })
}

// GET /api/messages/{id}/context returns { messages: Message[], message_id: number }
export interface MessageContextResponse {
  messages: Message[]
  message_id: number
}

export async function apiGetMessageContext(
  id: number,
  before = 10,
  after = 10
): Promise<MessageContextResponse> {
  return request<MessageContextResponse>('GET', `/messages/${id}/context`, undefined, {
    before,
    after,
  })
}

export async function apiDeleteMessage(id: number): Promise<void> {
  return request<void>('DELETE', `/messages/${id}`)
}

// ---- Reports ----
// Go returns db.Report; int64 fields → numbers

export interface Report {
  id: number
  reporter_id?: number
  reporter_username?: string
  reported_user_id?: number
  reported_username?: string
  reported_message_id?: number
  category_id: number
  category_name: string
  description: string
  status: 'open' | 'resolved' | 'dismissed'
  resolved_by?: number
  resolution_note?: string
  created_at: string
  updated_at: string
}

// GET /api/reports returns []Report directly
export async function apiGetReports(
  status?: string,
  limit = 50,
  offset = 0
): Promise<Report[]> {
  return request<Report[]>('GET', '/reports', undefined, { status, limit, offset })
}

// GET /api/reports/{id} returns { report, context?, target_message_id? }
export interface ReportDetailResponse {
  report: Report
  context?: Message[]
  target_message_id?: number
}

export async function apiGetReport(id: number): Promise<ReportDetailResponse> {
  return request<ReportDetailResponse>('GET', `/reports/${id}`)
}

export async function apiResolveReport(
  id: number,
  status: 'resolved' | 'dismissed',
  note?: string
): Promise<void> {
  return request<void>('POST', `/reports/${id}/resolve`, { status, note })
}

// ---- Servers ----
// Go returns db.Server; owner_id is int64 → number
// Note: no owner_username or member_count in db.Server

export interface Server {
  id: number
  name: string
  owner_id: number
  created_at: string
  updated_at: string
}

// GET /api/servers returns []Server directly
export async function apiGetServers(q?: string, limit = 50, offset = 0): Promise<Server[]> {
  return request<Server[]>('GET', '/servers', undefined, { q, limit, offset })
}

export async function apiDisbandServer(id: number): Promise<{ message: string; members_notified: number }> {
  return request<{ message: string; members_notified: number }>('DELETE', `/servers/${id}`)
}

// ---- Categories ----

export interface Category {
  id: number
  name: string
  created_at: string
}

export async function apiGetCategories(): Promise<Category[]> {
  return request<Category[]>('GET', '/categories')
}

export async function apiCreateCategory(name: string): Promise<Category> {
  return request<Category>('POST', '/categories', { name })
}

export async function apiDeleteCategory(id: number): Promise<void> {
  return request<void>('DELETE', `/categories/${id}`)
}
