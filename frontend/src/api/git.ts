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

export interface GitBranch {
  name: string;
  sha: string;
  is_default: boolean;
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

export async function getBranches(
  provider: GitProvider,
  owner: string,
  repo: string,
): Promise<GitBranch[]> {
  const q = new URLSearchParams({ owner, repo }).toString();
  return apiClient.get<GitBranch[]>(`/git/${provider}/branches?${q}`);
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
 * Match GitHub URLs we know how to render in chat. Three shapes:
 *   - https://github.com/{owner}/{repo}           — bare repo
 *   - https://github.com/{owner}/{repo}/tree/{ref}[/{path}]
 *   - https://github.com/{owner}/{repo}/blob/{ref}/{path}
 *
 * `ref` is captured as a single path segment. Branches that contain slashes
 * (e.g. `release/v2`) collide with `path` ambiguously without contacting
 * GitHub to disambiguate; those URLs fall through to plain links rather
 * than mis-parse. Issue/PR/commit URLs intentionally do not match — they
 * are P3 (separate spec).
 */
export const GITHUB_REPO_URL_RE =
  /https?:\/\/github\.com\/([A-Za-z0-9._-]+)\/([A-Za-z0-9._-]+)(?:\/(tree|blob)\/([A-Za-z0-9._\-]+)(?:\/([^\s?#]+?))?)?\/?(?=\s|$|[?#])/gi;

export interface ParsedRepoLink {
  provider: GitProvider;
  owner: string;
  repo: string;
  /** Branch / tag / SHA — present only when the URL was a tree/blob URL. */
  ref?: string;
  /** Path within the repo. Refers to a directory for `tree/`, file for `blob/`. */
  path?: string;
  /** True for `blob/` URLs (file), false for `tree/` URLs (directory). */
  isFile?: boolean;
}

/** Extract every supported GitHub link from a message body, deduped. */
export function extractRepoLinks(content: string): ParsedRepoLink[] {
  const seen = new Set<string>();
  const out: ParsedRepoLink[] = [];
  for (const m of content.matchAll(GITHUB_REPO_URL_RE)) {
    // Strip a trailing ".git" if pasted from a clone URL.
    const repo = m[2].replace(/\.git$/, '');
    const kind = m[3] as 'tree' | 'blob' | undefined;
    const ref = m[4];
    const path = m[5];
    // Dedup by (owner, repo) only — multiple posts of the same repo at
    // different paths still produce a single embed; the LAST occurrence wins
    // on ref/path so the most specific link in the message drives the embed.
    const key = `github:${m[1]}/${repo}`;
    const link: ParsedRepoLink = { provider: 'github', owner: m[1], repo };
    if (kind && ref) {
      link.ref = ref;
      if (path) link.path = path;
      link.isFile = kind === 'blob';
    }
    if (seen.has(key)) {
      // Replace earlier (less specific) entry if this one carries a path.
      if (link.ref || link.path) {
        const idx = out.findIndex(o => o.owner === link.owner && o.repo === link.repo);
        if (idx >= 0) out[idx] = link;
      }
    } else {
      seen.add(key);
      out.push(link);
    }
  }
  return out;
}
