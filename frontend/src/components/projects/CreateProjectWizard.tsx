import React, { useEffect, useState } from 'react';
import { Modal } from '../ui/Modal';
import { createProject, listPresets } from '../../api/projects';
import type { Project, ProjectPreset, ProjectSkillLevel } from '../../api/types';
import './Projects.css';

interface Props {
  isOpen: boolean;
  serverId: string;
  onClose: () => void;
  onCreated: (p: Project) => void;
}

const SKILL_LEVELS: { value: ProjectSkillLevel; label: string }[] = [
  { value: 'beginner', label: 'Beginner — explain things, ask before destructive ops' },
  { value: 'intermediate', label: 'Intermediate — comfortable with most tools' },
  { value: 'expert', label: 'Expert — terse, assume context' },
  { value: 'auto', label: 'Auto — let Claude decide' },
  { value: 'custom', label: 'Custom — I will write my own preamble' },
];

// A1.0 wizard: single step, manual CLAUDE.md textarea. Synthesis comes in A2.0.
export const CreateProjectWizard: React.FC<Props> = ({ isOpen, serverId, onClose, onCreated }) => {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [skillLevel, setSkillLevel] = useState<ProjectSkillLevel>('auto');
  const [presetSlug, setPresetSlug] = useState('');
  const [claudeMd, setClaudeMd] = useState('');
  const [presets, setPresets] = useState<ProjectPreset[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!isOpen) return;
    listPresets().then(setPresets).catch((e) => {
      console.error('failed to load presets', e);
    });
  }, [isOpen]);

  // Reset on open
  useEffect(() => {
    if (isOpen) {
      setName('');
      setDescription('');
      setSkillLevel('auto');
      setPresetSlug('');
      setClaudeMd('');
      setError(null);
    }
  }, [isOpen]);

  const submit = async () => {
    if (!name.trim()) {
      setError('Name is required.');
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const project = await createProject({
        server_id: serverId,
        name: name.trim(),
        description: description.trim(),
        skill_level: skillLevel,
        preset_slug: presetSlug || undefined,
        claude_md: claudeMd,
      });
      onCreated(project);
      onClose();
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Failed to create project.';
      setError(msg);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Create project">
      <div className="wizard-form">
        {error && <div className="wizard-error">{error}</div>}

        <label>
          Name
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            maxLength={80}
            placeholder="My new project"
            autoFocus
          />
        </label>

        <label>
          Description (optional)
          <input
            type="text"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="One-line summary"
          />
        </label>

        <label>
          Preset (optional)
          <select value={presetSlug} onChange={(e) => setPresetSlug(e.target.value)}>
            <option value="">No preset</option>
            {presets.map((p) => (
              <option key={p.slug} value={p.slug}>{p.name}</option>
            ))}
          </select>
        </label>

        <label>
          Skill level
          <select value={skillLevel} onChange={(e) => setSkillLevel(e.target.value as ProjectSkillLevel)}>
            {SKILL_LEVELS.map((s) => (
              <option key={s.value} value={s.value}>{s.label}</option>
            ))}
          </select>
        </label>

        <label>
          CLAUDE.md (manual for now — synthesis lands in A2.0)
          <textarea
            value={claudeMd}
            onChange={(e) => setClaudeMd(e.target.value)}
            placeholder={'# Project notes\n\nClaude will read this when working in this project.'}
          />
        </label>

        <div className="wizard-actions">
          <button className="secondary" onClick={onClose} disabled={submitting}>Cancel</button>
          <button className="primary" onClick={submit} disabled={submitting || !name.trim()}>
            {submitting ? 'Creating…' : 'Create project'}
          </button>
        </div>
      </div>
    </Modal>
  );
};
