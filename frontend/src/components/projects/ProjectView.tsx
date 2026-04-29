import React, { useEffect, useState } from 'react';
import { Github, X as XIcon, History, Check, AlertCircle, Loader2 } from 'lucide-react';
import {
  updateClaudeMD,
  deleteProject,
  updateProject,
  getClaudeMDVersions,
} from '../../api/projects';
import { getRepo } from '../../api/git';
import type { Project, ProjectClaudeMDVersion, ProjectRepo } from '../../api/types';
import './Projects.css';

interface Props {
  project: Project;
  onUpdated: (p: Project) => void;
  onDeleted: (id: string) => void;
}

// Mirrors the wizard's parser. Could be extracted if a third caller appears.
function parseGithubInput(s: string): { owner: string; repo: string } | null {
  const trimmed = s.trim();
  if (!trimmed) return null;
  const url = trimmed.match(/^https?:\/\/github\.com\/([A-Za-z0-9._-]+)\/([A-Za-z0-9._-]+)(?:[/?#].*)?$/i);
  if (url) return { owner: url[1], repo: url[2].replace(/\.git$/i, '') };
  const slash = trimmed.match(/^([A-Za-z0-9._-]+)\/([A-Za-z0-9._-]+)$/);
  if (slash) return { owner: slash[1], repo: slash[2].replace(/\.git$/i, '') };
  return null;
}

const RepoSection: React.FC<{
  project: Project;
  onUpdated: (p: Project) => void;
}> = ({ project, onUpdated }) => {
  const [editing, setEditing] = useState(false);
  const [input, setInput] = useState('');
  const [status, setStatus] = useState<'idle' | 'checking' | 'ok' | 'invalid' | 'notfound'>('idle');
  const [verified, setVerified] = useState<ProjectRepo | null>(null);
  const [saving, setSaving] = useState(false);
  const linked = project.repos && project.repos[0];

  useEffect(() => {
    if (!editing) return;
    if (!input.trim()) { setStatus('idle'); setVerified(null); return; }
    const parsed = parseGithubInput(input);
    if (!parsed) { setStatus('invalid'); setVerified(null); return; }
    setStatus('checking');
    const handle = setTimeout(async () => {
      try {
        await getRepo('github', parsed.owner, parsed.repo);
        setStatus('ok');
        setVerified({ provider: 'github', owner: parsed.owner, repo: parsed.repo });
      } catch {
        setStatus('notfound');
        setVerified(null);
      }
    }, 500);
    return () => clearTimeout(handle);
  }, [input, editing]);

  const save = async () => {
    if (!verified) return;
    setSaving(true);
    try {
      const updated = await updateProject(project.id, { repos: [verified] });
      onUpdated(updated);
      setEditing(false);
      setInput('');
    } finally {
      setSaving(false);
    }
  };

  const remove = async () => {
    setSaving(true);
    try {
      const updated = await updateProject(project.id, { repos: [] });
      onUpdated(updated);
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <h3>
        Repository
        {!editing && (
          <button className="section-action" onClick={() => { setEditing(true); setInput(linked ? `${linked.owner}/${linked.repo}` : ''); }}>
            {linked ? 'Change' : 'Link'}
          </button>
        )}
      </h3>

      {editing ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="https://github.com/owner/repo  or  owner/repo"
            className="claude-md-textarea"
            style={{ minHeight: 0, padding: '8px 10px', fontFamily: 'inherit', fontSize: 14 }}
            autoFocus
          />
          {status === 'checking' && <span className="wizard-repo-status"><Loader2 size={12} /> Checking…</span>}
          {status === 'ok' && verified && <span className="wizard-repo-status ok"><Check size={12} /> {verified.owner}/{verified.repo} verified</span>}
          {status === 'invalid' && <span className="wizard-repo-status"><AlertCircle size={12} /> Doesn't look like a GitHub URL or owner/repo.</span>}
          {status === 'notfound' && <span className="wizard-repo-status"><AlertCircle size={12} /> Repo not found or not accessible.</span>}
          <div className="project-view-actions">
            <button className="primary" onClick={save} disabled={saving || status !== 'ok'}>{saving ? 'Saving…' : 'Save'}</button>
            <button onClick={() => { setEditing(false); setInput(''); }} disabled={saving}>Cancel</button>
            {linked && !saving && (
              <button className="danger" onClick={remove}>Unlink current</button>
            )}
          </div>
        </div>
      ) : linked ? (
        <div className="project-repo-row">
          <Github size={16} />
          <a href={`https://github.com/${linked.owner}/${linked.repo}`} target="_blank" rel="noreferrer">
            {linked.owner}/{linked.repo}
          </a>
          <button className="project-repo-remove" onClick={remove} disabled={saving} title="Unlink">
            <XIcon size={14} />
          </button>
        </div>
      ) : (
        <div className="project-repo-empty">No repository linked. Click "Link" to attach a GitHub repo to this project.</div>
      )}
    </>
  );
};

const VersionHistory: React.FC<{ projectId: string; onClose: () => void }> = ({ projectId, onClose }) => {
  const [versions, setVersions] = useState<ProjectClaudeMDVersion[] | null>(null);
  const [expandedId, setExpandedId] = useState<number | null>(null);

  useEffect(() => {
    let cancelled = false;
    getClaudeMDVersions(projectId)
      .then((vs) => { if (!cancelled) setVersions(vs); })
      .catch(() => { if (!cancelled) setVersions([]); });
    return () => { cancelled = true; };
  }, [projectId]);

  return (
    <>
      <h3>
        CLAUDE.md history
        <button className="section-action" onClick={onClose}>Close</button>
      </h3>
      {versions === null ? (
        <div className="projects-empty">Loading…</div>
      ) : versions.length === 0 ? (
        <div className="project-repo-empty">No history yet.</div>
      ) : (
        <div className="version-history-list">
          {versions.map((v) => (
            <div key={v.id} className="version-history-item" onClick={() => setExpandedId(expandedId === v.version ? null : v.version)}>
              <div className="version-history-item-head">
                <span className="version-tag">v{v.version}</span>
                <span>{new Date(v.created_at).toLocaleString()}</span>
              </div>
              {expandedId === v.version && <pre className="version-history-content">{v.content || '(empty)'}</pre>}
            </div>
          ))}
        </div>
      )}
    </>
  );
};

export const ProjectView: React.FC<Props> = ({ project, onUpdated, onDeleted }) => {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(project.claude_md);
  const [saving, setSaving] = useState(false);
  const [showHistory, setShowHistory] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Reset draft when project changes (e.g. WS update from another client)
  useEffect(() => {
    if (!editing) setDraft(project.claude_md);
  }, [project.claude_md, editing]);

  const startEdit = () => {
    setDraft(project.claude_md);
    setEditing(true);
    setError(null);
  };

  const cancel = () => {
    setEditing(false);
    setDraft(project.claude_md);
    setError(null);
  };

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      const updated = await updateClaudeMD(project.id, draft);
      onUpdated(updated);
      setEditing(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save.');
    } finally {
      setSaving(false);
    }
  };

  const remove = async () => {
    if (!window.confirm(`Delete project "${project.name}"? This cannot be undone.`)) return;
    try {
      await deleteProject(project.id);
      onDeleted(project.id);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to delete.');
    }
  };

  return (
    <div className="project-view">
      <div className="project-view-meta">
        <h1>{project.name}</h1>
        {project.description && <div className="project-view-meta-desc">{project.description}</div>}
        <div className="project-view-meta-row">
          <span className="project-card-meta-pill">{project.skill_level}</span>
          {project.preset_name && <span className="project-card-meta-pill">{project.preset_name}</span>}
          <span>v{project.version_count} of CLAUDE.md</span>
        </div>
      </div>

      <div className="project-view-section">
        <RepoSection project={project} onUpdated={onUpdated} />

        <div style={{ height: 18 }} />

        {showHistory ? (
          <VersionHistory projectId={project.id} onClose={() => setShowHistory(false)} />
        ) : (
          <>
            <h3>
              CLAUDE.md
              {!editing && project.version_count > 1 && (
                <button className="section-action" onClick={() => setShowHistory(true)}>
                  <History size={12} style={{ verticalAlign: 'middle' }} /> View history ({project.version_count})
                </button>
              )}
            </h3>
            {error && <div className="wizard-error" style={{ marginBottom: 10 }}>{error}</div>}
            {editing ? (
              <textarea
                className="claude-md-textarea"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
              />
            ) : (
              <pre className="claude-md-display">{project.claude_md || '(empty — click Edit to add notes)'}</pre>
            )}

            <div className="project-view-actions">
              {editing ? (
                <>
                  <button className="primary" onClick={save} disabled={saving}>
                    {saving ? 'Saving…' : 'Save'}
                  </button>
                  <button onClick={cancel} disabled={saving}>Cancel</button>
                </>
              ) : (
                <>
                  <button className="primary" onClick={startEdit}>Edit CLAUDE.md</button>
                  <button className="danger" onClick={remove}>Delete project</button>
                </>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
};
