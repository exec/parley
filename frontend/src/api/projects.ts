import { apiClient } from './client';
import { Project, ProjectPreset, ProjectClaudeMDVersion, ProjectRepo, ProjectSkillLevel } from './types';

// ── Create / read / update / delete ───────────────────────────────────────────

export interface CreateProjectData {
  server_id: string;
  name: string;
  description?: string;
  claude_md?: string;
  skill_level?: ProjectSkillLevel;
  preset_slug?: string;
  vc_channel_id?: string;
  repos?: ProjectRepo[];
  skills?: string[];
}

// Field semantics on update:
//   - omit field    → leave alone
//   - empty string  → clear (preset_slug, vc_channel_id only)
//   - non-empty     → set to value
//   - description "" is allowed and clears (the field is required NOT NULL but DEFAULT '')
export interface UpdateProjectData {
  name?: string;
  description?: string;
  skill_level?: ProjectSkillLevel;
  preset_slug?: string;
  vc_channel_id?: string;
  repos?: ProjectRepo[];
  skills?: string[];
}

export async function createProject(data: CreateProjectData): Promise<Project> {
  return apiClient.post<Project>('/projects', data);
}

export async function getServerProjects(serverId: string): Promise<Project[]> {
  return apiClient.get<Project[]>(`/servers/${serverId}/projects`);
}

export async function getProject(projectId: string): Promise<Project> {
  return apiClient.get<Project>(`/projects/${projectId}`);
}

export async function updateProject(projectId: string, data: UpdateProjectData): Promise<Project> {
  return apiClient.patch<Project>(`/projects/${projectId}`, data);
}

export async function updateClaudeMD(projectId: string, content: string): Promise<Project> {
  return apiClient.patch<Project>(`/projects/${projectId}/claude-md`, { content });
}

export async function deleteProject(projectId: string): Promise<void> {
  return apiClient.delete<void>(`/projects/${projectId}`);
}

// ── Presets ───────────────────────────────────────────────────────────────────

export async function listPresets(): Promise<ProjectPreset[]> {
  return apiClient.get<ProjectPreset[]>('/projects/presets');
}

// ── CLAUDE.md version history ─────────────────────────────────────────────────

export async function getClaudeMDVersions(projectId: string): Promise<ProjectClaudeMDVersion[]> {
  return apiClient.get<ProjectClaudeMDVersion[]>(`/projects/${projectId}/claude-md/versions`);
}
