import React, { useState } from 'react';
import { updateClaudeMD, deleteProject } from '../../api/projects';
import type { Project } from '../../api/types';
import './Projects.css';

interface Props {
  project: Project;
  onUpdated: (p: Project) => void;
  onDeleted: (id: string) => void;
}

export const ProjectView: React.FC<Props> = ({ project, onUpdated, onDeleted }) => {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(project.claude_md);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Reset draft when project changes (e.g. WS update from another client)
  React.useEffect(() => {
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
        <h1 style={{ margin: 0, fontSize: 20 }}>{project.name}</h1>
        {project.description && <div style={{ color: 'var(--parley-text-muted)' }}>{project.description}</div>}
        <div className="project-view-meta-row">
          <span className="project-card-meta-pill">{project.skill_level}</span>
          {project.preset_name && <span className="project-card-meta-pill">{project.preset_name}</span>}
          {project.repos && project.repos.length > 0 && (
            <span className="project-card-meta-pill">
              {project.repos[0].provider}:{project.repos[0].owner}/{project.repos[0].repo}
            </span>
          )}
          <span>v{project.version_count} of CLAUDE.md</span>
        </div>
      </div>

      <div className="project-view-section">
        <h3>CLAUDE.md</h3>
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
      </div>
    </div>
  );
};
