import React, { useState, useCallback, useRef, useEffect } from 'react';
import { X, Upload } from 'lucide-react';
import { Button } from '../ui/Button';
import { BinChannelTag } from '../../api/types';
import { createPost } from '../../api/bin';
import { languageFromFilename, isCodeFile } from '../../lib/shiki';
import './CreatePostModal.css';

interface FileEntry {
  filename: string;
  language: string;
  content: string;
}

interface CreatePostModalProps {
  isOpen: boolean;
  channelId: string;
  availableTags: BinChannelTag[];
  onClose: () => void;
  onCreated: (postId: string) => void;
}

function makeEmptyFile(): FileEntry {
  return { filename: '', language: '', content: '' };
}

export const CreatePostModal: React.FC<CreatePostModalProps> = ({
  isOpen,
  channelId,
  availableTags,
  onClose,
  onCreated,
}) => {
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [files, setFiles] = useState<FileEntry[]>([makeEmptyFile()]);
  const [activeFileIndex, setActiveFileIndex] = useState(0);
  const [selectedTagNames, setSelectedTagNames] = useState<Set<string>>(new Set());
  const [customTags, setCustomTags] = useState<string[]>([]);
  const [tagInput, setTagInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [dragging, setDragging] = useState(false);
  const titleRef = useRef<HTMLInputElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleUploadFiles = useCallback((fileList: FileList | File[]) => {
    const arr = Array.from(fileList).filter(f => isCodeFile(f.name));
    if (arr.length === 0) return;
    arr.forEach(file => {
      const reader = new FileReader();
      reader.onload = e => {
        const content = e.target?.result as string ?? '';
        setFiles(prev => {
          const isOnlyEmpty = prev.length === 1 && !prev[0].filename && !prev[0].content;
          const base = isOnlyEmpty ? [] : prev;
          const next = [...base, {
            filename: file.name,
            language: languageFromFilename(file.name),
            content,
          }];
          setActiveFileIndex(next.length - 1);
          return next;
        });
      };
      // readAsText is intentional — bin is a code-sharing feature.
      // Binary files are excluded upstream by the isCodeFile extension filter.
      reader.readAsText(file);
    });
  }, []);

  // Focus title when modal opens
  useEffect(() => {
    if (isOpen) {
      setTimeout(() => titleRef.current?.focus(), 50);
    }
  }, [isOpen]);

  // Close on Escape; paste files
  useEffect(() => {
    if (!isOpen) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') handleClose(); };
    const onPaste = (e: ClipboardEvent) => {
      if (e.clipboardData?.files?.length) handleUploadFiles(e.clipboardData.files);
    };
    document.addEventListener('keydown', onKey);
    document.addEventListener('paste', onPaste);
    return () => {
      document.removeEventListener('keydown', onKey);
      document.removeEventListener('paste', onPaste);
    };
  }, [isOpen, handleUploadFiles]);

  const resetForm = useCallback(() => {
    setTitle('');
    setDescription('');
    setFiles([makeEmptyFile()]);
    setActiveFileIndex(0);
    setSelectedTagNames(new Set());
    setCustomTags([]);
    setTagInput('');
    setError('');
  }, []);

  const handleClose = () => {
    resetForm();
    onClose();
  };

  // ---- File management ----

  const updateFile = (index: number, field: keyof FileEntry, value: string) => {
    setFiles(prev => {
      const next = [...prev];
      next[index] = { ...next[index], [field]: value };
      if (field === 'filename' && !next[index].language) {
        const detected = languageFromFilename(value);
        if (detected) next[index] = { ...next[index], language: detected };
      }
      return next;
    });
  };

  const addFile = () => {
    setFiles(prev => {
      const next = [...prev, makeEmptyFile()];
      setActiveFileIndex(next.length - 1);
      return next;
    });
  };

  const removeFile = (index: number) => {
    if (files.length === 1) return;
    setFiles(prev => {
      const next = prev.filter((_, i) => i !== index);
      setActiveFileIndex(i => Math.min(i, next.length - 1));
      return next;
    });
  };

  // ---- Tag management ----

  const toggleAdminTag = (tagName: string) => {
    setSelectedTagNames(prev => {
      const next = new Set(prev);
      if (next.has(tagName)) next.delete(tagName);
      else next.add(tagName);
      return next;
    });
  };

  const handleTagInputKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      const val = tagInput.trim().toLowerCase().replace(/\s+/g, '-');
      if (val && !customTags.includes(val) && !selectedTagNames.has(val)) {
        setCustomTags(prev => [...prev, val]);
      }
      setTagInput('');
    }
  };

  const removeCustomTag = (tag: string) => {
    setCustomTags(prev => prev.filter(t => t !== tag));
  };

  // ---- Submit ----

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!title.trim()) {
      setError('Title is required');
      return;
    }

    const validFiles = files.filter(f => f.filename.trim() || f.content.trim());
    if (validFiles.length === 0) {
      setError('At least one file with a filename or content is required');
      return;
    }

    setLoading(true);
    setError('');

    const allTags = [...Array.from(selectedTagNames), ...customTags];

    try {
      const post = await createPost(channelId, {
        title: title.trim(),
        description: description.trim() || undefined,
        tags: allTags.length > 0 ? allTags : undefined,
        files: validFiles.map((f, i) => ({
          filename: f.filename.trim() || `file-${i + 1}`,
          language: f.language.trim(),
          content: f.content,
          position: i,
        })),
      });
      resetForm();
      onCreated(post.id);
    } catch (err) {
      setError((err as any)?.message || 'Failed to create post');
    } finally {
      setLoading(false);
    }
  };

  if (!isOpen) return null;

  const activeFile = files[activeFileIndex] ?? files[0];

  const allTags = [...Array.from(selectedTagNames), ...customTags];

  return (
    <div
      className={`cpm-overlay${dragging ? ' cpm-dragging' : ''}`}
      onClick={e => { if (e.target === e.currentTarget) handleClose(); }}
      onDragOver={e => { e.preventDefault(); setDragging(true); }}
      onDragLeave={e => { if (!e.currentTarget.contains(e.relatedTarget as Node)) setDragging(false); }}
      onDrop={e => { e.preventDefault(); setDragging(false); handleUploadFiles(e.dataTransfer.files); }}
    >
      <div className="cpm-window">
        {/* Title bar */}
        <div className="cpm-titlebar">
          <span className="cpm-titlebar-label">New Post</span>
          <button className="cpm-close-btn" onClick={handleClose} aria-label="Close">
            <X size={16} />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="cpm-body">
          {/* Metadata row */}
          <div className="cpm-meta">
            <div className="cpm-meta-left">
              <div className="cpm-field cpm-field-title">
                <label className="cpm-label">
                  Title <span className="cpm-required">*</span>
                </label>
                <input
                  ref={titleRef}
                  className="cpm-input"
                  type="text"
                  value={title}
                  onChange={e => setTitle(e.target.value)}
                  placeholder="Post title"
                  disabled={loading}
                />
              </div>
              <div className="cpm-field cpm-field-desc">
                <label className="cpm-label">
                  Description <span className="cpm-optional">optional</span>
                </label>
                <textarea
                  className="cpm-input cpm-desc-textarea"
                  value={description}
                  onChange={e => setDescription(e.target.value)}
                  placeholder="Brief description — supports markdown"
                  disabled={loading}
                  rows={2}
                />
              </div>
            </div>

            <div className="cpm-meta-right">
              <div className="cpm-field">
                <label className="cpm-label">Tags</label>
                <div className="cpm-tags-area">
                  {availableTags.length > 0 && (
                    <div className="cpm-tag-pills">
                      {availableTags.map(tag => (
                        <button
                          key={tag.id}
                          type="button"
                          className={`cpm-tag-pill${selectedTagNames.has(tag.name) ? ' selected' : ''}`}
                          style={{ borderColor: tag.color, color: tag.color }}
                          onClick={() => toggleAdminTag(tag.name)}
                          disabled={loading}
                        >
                          {tag.name}
                        </button>
                      ))}
                    </div>
                  )}
                  <div className="cpm-tag-input-row">
                    <input
                      className="cpm-input cpm-tag-input"
                      type="text"
                      value={tagInput}
                      onChange={e => setTagInput(e.target.value)}
                      onKeyDown={handleTagInputKeyDown}
                      placeholder="Custom tag, press Enter"
                      disabled={loading}
                    />
                  </div>
                  {allTags.length > 0 && (
                    <div className="cpm-selected-tags">
                      {Array.from(selectedTagNames).map(name => {
                        const t = availableTags.find(at => at.name === name);
                        return (
                          <span key={name} className="cpm-selected-tag" style={t ? { borderColor: t.color, color: t.color } : {}}>
                            {name}
                            <button type="button" className="cpm-tag-remove" onClick={() => toggleAdminTag(name)}>×</button>
                          </span>
                        );
                      })}
                      {customTags.map(tag => (
                        <span key={tag} className="cpm-selected-tag cpm-custom-tag">
                          {tag}
                          <button type="button" className="cpm-tag-remove" onClick={() => removeCustomTag(tag)}>×</button>
                        </span>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            </div>
          </div>

          {/* Editor area */}
          <div className="cpm-editor">
            {/* File tabs */}
            <div className="cpm-tabs">
              {files.map((file, i) => (
                <button
                  key={i}
                  type="button"
                  className={`cpm-tab${i === activeFileIndex ? ' active' : ''}`}
                  onClick={() => setActiveFileIndex(i)}
                >
                  <span className="cpm-tab-name">
                    {file.filename || `untitled-${i + 1}`}
                  </span>
                  {files.length > 1 && (
                    <span
                      className="cpm-tab-close"
                      onClick={e => { e.stopPropagation(); removeFile(i); }}
                      role="button"
                      aria-label="Remove file"
                    >
                      ×
                    </span>
                  )}
                </button>
              ))}
              <button type="button" className="cpm-tab-add" onClick={addFile} disabled={loading} title="Add file">
                +
              </button>
              <button type="button" className="cpm-tab-upload" onClick={() => fileInputRef.current?.click()} disabled={loading} title="Upload files">
                <Upload size={13} />
              </button>
              <input
                ref={fileInputRef}
                type="file"
                multiple
                style={{ display: 'none' }}
                onChange={e => { if (e.target.files) { handleUploadFiles(e.target.files); e.target.value = ''; } }}
              />
            </div>

            {/* Active file editor */}
            <div className="cpm-file-meta">
              <input
                className="cpm-input cpm-filename-input"
                type="text"
                value={activeFile.filename}
                onChange={e => updateFile(activeFileIndex, 'filename', e.target.value)}
                placeholder="filename.ts"
                disabled={loading}
              />
              <input
                className="cpm-input cpm-lang-input"
                type="text"
                value={activeFile.language}
                onChange={e => updateFile(activeFileIndex, 'language', e.target.value)}
                placeholder="language"
                disabled={loading}
              />
            </div>

            <textarea
              className="cpm-code-editor"
              value={activeFile.content}
              onChange={e => updateFile(activeFileIndex, 'content', e.target.value)}
              placeholder="Paste or type your code here..."
              spellCheck={false}
              disabled={loading}
            />
          </div>

          {/* Footer */}
          <div className="cpm-footer">
            <div className="cpm-footer-left">
              {error && <span className="cpm-error">{error}</span>}
              <span className="cpm-file-count">{files.length} file{files.length !== 1 ? 's' : ''}</span>
            </div>
            <div className="cpm-footer-actions">
              <Button type="button" variant="secondary" onClick={handleClose} disabled={loading}>
                Cancel
              </Button>
              <Button type="submit" variant="primary" loading={loading}>
                Create Post
              </Button>
            </div>
          </div>
        </form>
      </div>
    </div>
  );
};

export default CreatePostModal;
