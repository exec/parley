import { apiClient } from './client';
import { BinPost, BinPostVersion, BinLineComment, BinChannelTag } from './types';

// ── Post CRUD ─────────────────────────────────────────────────────────────────

export interface CreatePostData {
  title: string;
  description?: string;
  tags?: string[];
  files: { filename: string; language: string; content: string; position: number }[];
}

export interface EditPostData {
  title?: string;
  description?: string;
  tags?: string[];
  files?: { filename: string; language: string; content: string; position: number }[];
  version_description?: string;
}

export interface ListPostsParams {
  limit?: number;
  offset?: number;
  tag?: string;
  search?: string;
  sort?: string;
}

export async function createPost(channelId: string, data: CreatePostData): Promise<BinPost> {
  return apiClient.post<BinPost>(`/channels/${channelId}/posts`, data);
}

export async function listPosts(channelId: string, params?: ListPostsParams): Promise<BinPost[]> {
  const queryParams = new URLSearchParams();
  if (params?.limit) queryParams.append('limit', params.limit.toString());
  if (params?.offset) queryParams.append('offset', params.offset.toString());
  if (params?.tag) queryParams.append('tag', params.tag);
  if (params?.search) queryParams.append('search', params.search);
  if (params?.sort) queryParams.append('sort', params.sort);
  const queryString = queryParams.toString();
  const endpoint = `/channels/${channelId}/posts${queryString ? `?${queryString}` : ''}`;
  return apiClient.get<BinPost[]>(endpoint);
}

export async function getPost(postId: string): Promise<BinPost> {
  return apiClient.get<BinPost>(`/posts/${postId}`);
}

export async function editPost(postId: string, data: EditPostData): Promise<BinPost> {
  return apiClient.put<BinPost>(`/posts/${postId}`, data);
}

export async function deletePost(postId: string): Promise<void> {
  return apiClient.delete<void>(`/posts/${postId}`);
}

// ── Versions ──────────────────────────────────────────────────────────────────

export async function getVersions(postId: string): Promise<BinPostVersion[]> {
  return apiClient.get<BinPostVersion[]>(`/posts/${postId}/versions`);
}

export async function getVersion(postId: string, versionId: string): Promise<BinPostVersion> {
  return apiClient.get<BinPostVersion>(`/posts/${postId}/versions/${versionId}`);
}

// ── Line Comments ─────────────────────────────────────────────────────────────

export interface CreateLineCommentData {
  version_id: string;
  file_id: string;
  line_number: number;
  content: string;
  parent_id?: string;
}

export interface GetLineCommentsParams {
  version_id?: string;
  file_id?: string;
  line_number?: number;
}

export async function createLineComment(postId: string, data: CreateLineCommentData): Promise<BinLineComment> {
  return apiClient.post<BinLineComment>(`/posts/${postId}/line-comments`, data);
}

export async function getLineComments(postId: string, params?: GetLineCommentsParams): Promise<BinLineComment[]> {
  const queryParams = new URLSearchParams();
  if (params?.version_id) queryParams.append('version_id', params.version_id);
  if (params?.file_id) queryParams.append('file_id', params.file_id);
  if (params?.line_number !== undefined) queryParams.append('line_number', params.line_number.toString());
  const queryString = queryParams.toString();
  const endpoint = `/posts/${postId}/line-comments${queryString ? `?${queryString}` : ''}`;
  return apiClient.get<BinLineComment[]>(endpoint);
}

export async function updateLineComment(id: string, content: string): Promise<BinLineComment> {
  return apiClient.put<BinLineComment>(`/line-comments/${id}`, { content });
}

export async function deleteLineComment(id: string): Promise<void> {
  return apiClient.delete<void>(`/line-comments/${id}`);
}

// ── Tags ──────────────────────────────────────────────────────────────────────

export async function getTags(channelId: string): Promise<BinChannelTag[]> {
  return apiClient.get<BinChannelTag[]>(`/channels/${channelId}/tags`);
}

export async function createTag(channelId: string, name: string, color: string): Promise<BinChannelTag> {
  return apiClient.post<BinChannelTag>(`/channels/${channelId}/tags`, { name, color });
}

export async function deleteTag(channelId: string, tagId: string): Promise<void> {
  return apiClient.delete<void>(`/channels/${channelId}/tags/${tagId}`);
}
