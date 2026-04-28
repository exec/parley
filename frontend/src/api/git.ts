import { apiClient } from './client';

// Mirrors internal/gitprovider types verbatim.

export interface GitRepo {
  owner: string;
  name: string;
  description: string;
  default_branch: string;
  language?: string;
  owner_avatar_url?: string;
  html_url: string;
  stars: number;
  forks: number;
  private: boolean;
  pushed_at: string;  // RFC3339
  updated_at: string;
  latest_release?: GitRelease;
}

export interface GitTreeEntry {
  path: string;
  name: string;
  type: 'file' | 'dir' | 'symlink' | 'submodule';
  size: number;
  sha: string;
}

export interface GitBlob {
  path: string;
  sha: string;
  size: number;
  content_type: 'text' | 'binary';
  /** base64-encoded text bytes when content_type='text' and not truncated. */
  content?: string;
  html_url: string;
  truncated?: boolean;
}

export interface GitRelease {
  tag_name: string;
  name: string;
  body?: string;
  html_url: string;
  published_at: string;
}

export type GitProvider = 'github';

export async function getRepo(provider: GitProvider, owner: string, repo: string): Promise<GitRepo> {
  const q = new URLSearchParams({ owner, repo }).toString();
  return apiClient.get<GitRepo>(`/git/${provider}/repo?${q}`);
}

export async function getTree(
  provider: GitProvider,
  owner: string,
  repo: string,
  ref: string = '',
  path: string = '',
): Promise<GitTreeEntry[]> {
  const q = new URLSearchParams({ owner, repo });
  if (ref) q.set('ref', ref);
  if (path) q.set('path', path);
  return apiClient.get<GitTreeEntry[]>(`/git/${provider}/tree?${q.toString()}`);
}

export async function getBlob(
  provider: GitProvider,
  owner: string,
  repo: string,
  ref: string,
  path: string,
): Promise<GitBlob> {
  const q = new URLSearchParams({ owner, repo, path });
  if (ref) q.set('ref', ref);
  return apiClient.get<GitBlob>(`/git/${provider}/blob?${q.toString()}`);
}

export async function getReleases(
  provider: GitProvider,
  owner: string,
  repo: string,
  limit: number = 5,
): Promise<GitRelease[]> {
  const q = new URLSearchParams({ owner, repo, limit: String(limit) });
  return apiClient.get<GitRelease[]>(`/git/${provider}/releases?${q.toString()}`);
}

/**
 * Decode the base64 `content` field on a text Blob into a UTF-8 string.
 * Returns '' for binary, missing, or truncated blobs.
 */
export function decodeBlobText(blob: GitBlob): string {
  if (blob.content_type !== 'text' || !blob.content || blob.truncated) return '';
  try {
    const bytes = Uint8Array.from(atob(blob.content), c => c.charCodeAt(0));
    return new TextDecoder('utf-8').decode(bytes);
  } catch {
    return '';
  }
}

/**
 * Match a bare GitHub repo URL: https://github.com/{owner}/{repo} (with
 * optional trailing slash). Issue/PR/tree/blob URLs intentionally do NOT
 * match in V1 — they fall through to plain link rendering.
 */
export const GITHUB_REPO_URL_RE =
  /https?:\/\/github\.com\/([A-Za-z0-9._-]+)\/([A-Za-z0-9._-]+)\/?(?=\s|$|[?#])/gi;

export interface ParsedRepoLink {
  provider: GitProvider;
  owner: string;
  repo: string;
}

/** Extract all bare GitHub repo links from a message body, deduped. */
export function extractRepoLinks(content: string): ParsedRepoLink[] {
  const seen = new Set<string>();
  const out: ParsedRepoLink[] = [];
  for (const m of content.matchAll(GITHUB_REPO_URL_RE)) {
    // Strip a trailing ".git" if pasted from a clone URL.
    const repo = m[2].replace(/\.git$/, '');
    const key = `github:${m[1]}/${repo}`;
    if (!seen.has(key)) {
      seen.add(key);
      out.push({ provider: 'github', owner: m[1], repo });
    }
  }
  return out;
}
