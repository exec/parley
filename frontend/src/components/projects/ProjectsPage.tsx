import React, { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { ArrowLeft, Plus } from 'lucide-react';
import { getServerProjects, getProject } from '../../api/projects';
import type { Project } from '../../api/types';
import { CreateProjectWizard } from './CreateProjectWizard';
import { ProjectView } from './ProjectView';
import './Projects.css';

interface Props {
  serverId: string;
  projectId: string | null;
  onClose: () => void;
}

// Top-level page for a server's projects. URL drives whether we're on the
// list (projectId == null) or detail (projectId set).
export const ProjectsPage: React.FC<Props> = ({ serverId, projectId, onClose }) => {
  const navigate = useNavigate();
  const [projects, setProjects] = useState<Project[]>([]);
  const [activeProject, setActiveProject] = useState<Project | null>(null);
  const [loading, setLoading] = useState(true);
  const [showWizard, setShowWizard] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Load list whenever server changes.
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    getServerProjects(serverId)
      .then((list) => { if (!cancelled) { setProjects(list); setError(null); } })
      .catch((e) => { if (!cancelled) setError(e instanceof Error ? e.message : 'Failed to load projects.'); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [serverId]);

  // Load active project (full payload) when projectId in URL.
  useEffect(() => {
    if (!projectId) {
      setActiveProject(null);
      return;
    }
    let cancelled = false;
    getProject(projectId)
      .then((p) => { if (!cancelled) setActiveProject(p); })
      .catch((e) => { if (!cancelled) setError(e instanceof Error ? e.message : 'Failed to load project.'); });
    return () => { cancelled = true; };
  }, [projectId]);

  const onCreated = useCallback((p: Project) => {
    setProjects((prev) => [p, ...prev.filter((x) => x.id !== p.id)]);
  }, []);

  const onUpdated = useCallback((p: Project) => {
    setActiveProject(p);
    setProjects((prev) => prev.map((x) => (x.id === p.id ? p : x)));
  }, []);

  const onDeleted = useCallback((id: string) => {
    setProjects((prev) => prev.filter((x) => x.id !== id));
    setActiveProject(null);
    navigate(`/servers/${serverId}/projects`);
  }, [navigate, serverId]);

  // Subscribe to WS-driven project events dispatched by App.tsx so other
  // server members see real-time create/update/delete without refetching.
  useEffect(() => {
    const onCreate = (e: Event) => {
      const p = (e as CustomEvent<Project>).detail;
      if (p.server_id === serverId) {
        setProjects((prev) => prev.some((x) => x.id === p.id) ? prev : [p, ...prev]);
      }
    };
    const onUpdate = (e: Event) => {
      const p = (e as CustomEvent<Project>).detail;
      if (p.server_id !== serverId) return;
      setProjects((prev) => prev.map((x) => (x.id === p.id ? p : x)));
      setActiveProject((cur) => (cur && cur.id === p.id ? p : cur));
    };
    const onDelete = (e: Event) => {
      const evt = (e as CustomEvent<{ id: string; server_id: string }>).detail;
      if (evt.server_id !== serverId) return;
      setProjects((prev) => prev.filter((x) => x.id !== evt.id));
      setActiveProject((cur) => (cur && cur.id === evt.id ? null : cur));
    };
    window.addEventListener('parley:project_create', onCreate);
    window.addEventListener('parley:project_update', onUpdate);
    window.addEventListener('parley:project_delete', onDelete);
    return () => {
      window.removeEventListener('parley:project_create', onCreate);
      window.removeEventListener('parley:project_update', onUpdate);
      window.removeEventListener('parley:project_delete', onDelete);
    };
  }, [serverId]);

  const openProject = (id: string) => navigate(`/servers/${serverId}/projects/${id}`);
  const backToList = () => navigate(`/servers/${serverId}/projects`);

  return (
    <div className="projects-page">
      <div className="projects-header">
        <button className="projects-back-btn" onClick={projectId ? backToList : onClose}>
          <ArrowLeft size={16} /> {projectId ? 'All projects' : 'Back'}
        </button>
        <h1>{projectId ? (activeProject?.name ?? 'Project') : 'Projects'}</h1>
        {!projectId && (
          <button className="projects-create-btn" onClick={() => setShowWizard(true)}>
            <Plus size={16} /> New project
          </button>
        )}
      </div>

      <div className="projects-body">
        {error && <div className="wizard-error">{error}</div>}

        {projectId ? (
          activeProject ? (
            <ProjectView project={activeProject} onUpdated={onUpdated} onDeleted={onDeleted} />
          ) : (
            !error && <div className="projects-empty">Loading…</div>
          )
        ) : loading ? (
          <div className="projects-empty">Loading…</div>
        ) : projects.length === 0 ? (
          <div className="projects-empty">
            <h2>No projects yet</h2>
            <p>Projects are the entry point for AI-paired work in this server.</p>
            <button className="projects-create-btn" onClick={() => setShowWizard(true)}>
              <Plus size={16} /> Create your first project
            </button>
          </div>
        ) : (
          <div className="projects-grid">
            {projects.map((p) => (
              <div key={p.id} className="project-card" onClick={() => openProject(p.id)}>
                <div className="project-card-title">{p.name}</div>
                <div className="project-card-meta">
                  <span className="project-card-meta-pill">{p.skill_level}</span>
                  {p.preset_name && <span className="project-card-meta-pill">{p.preset_name}</span>}
                  <span>v{p.version_count}</span>
                </div>
                {p.description && <div className="project-card-desc">{p.description}</div>}
              </div>
            ))}
          </div>
        )}
      </div>

      <CreateProjectWizard
        isOpen={showWizard}
        serverId={serverId}
        onClose={() => setShowWizard(false)}
        onCreated={onCreated}
      />
    </div>
  );
};
