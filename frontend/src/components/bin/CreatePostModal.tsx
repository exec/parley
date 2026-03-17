import React, { useState, useCallback } from 'react';
import { Modal } from '../ui/Modal';
import { Button } from '../ui/Button';
import { BinChannelTag } from '../../api/types';
import { createPost } from '../../api/bin';
import { languageFromFilename } from '../../lib/shiki';
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
  const [selectedTagNames, setSelectedTagNames] = useState<Set<string>>(new Set());
  const [customTags, setCustomTags] = useState<string[]>([]);
  const [tagInput, setTagInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const resetForm = useCallback(() => {
    setTitle('');
    setDescription('');
    setFiles([makeEmptyFile()]);
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
      // Auto-detect language when filename changes
      if (field === 'filename' && !next[index].language) {
        const detected = languageFromFilename(value);
        if (detected) {
          next[index] = { ...next[index], language: detected };
        }
      }
      return next;
    });
  };

  const addFile = () => {
    setFiles(prev => [...prev, makeEmptyFile()]);
  };

  const removeFile = (index: number) => {
    setFiles(prev => prev.filter((_, i) => i !== index));
  };

  // ---- Tag management ----

  const toggleAdminTag = (tagName: string) => {
    setSelectedTagNames(prev => {
      const next = new Set(prev);
      if (next.has(tagName)) {
        next.delete(tagName);
      } else {
        next.add(tagName);
      }
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

    const allTags = [
      ...Array.from(selectedTagNames),
      ...customTags,
    ];

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
      setError(err instanceof Error ? err.message : 'Failed to create post');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={handleClose} title="New Post">
      <div className="create-post-modal">
        <form onSubmit={handleSubmit} className="modal-form">
          {error && <div className="modal-error">{error}</div>}

          {/* Title */}
          <div className="form-group">
            <label className="form-label">Title <span style={{ color: 'var(--parley-danger)', marginLeft: 2 }}>*</span></label>
            <input
              className="form-input"
              type="text"
              value={title}
              onChange={e => setTitle(e.target.value)}
              placeholder="Post title"
              autoFocus
              disabled={loading}
            />
          </div>

          {/* Description */}
          <div className="form-group">
            <label className="form-label">
              Description <span style={{ color: 'var(--parley-text-dim)', fontWeight: 400, textTransform: 'none', letterSpacing: 0 }}>optional</span>
            </label>
            <textarea
              className="form-input create-post-desc-textarea"
              value={description}
              onChange={e => setDescription(e.target.value)}
              placeholder="Describe your post — supports markdown and ``` code blocks ```"
              disabled={loading}
            />
          </div>

          {/* Files */}
          <div className="form-group">
            <label className="form-label">Files</label>
            <div className="create-post-files">
              {files.map((file, index) => (
                <div key={index} className="create-post-file-entry">
                  <div className="create-post-file-header">
                    <span className="create-post-file-number">File {index + 1}</span>
                    <div className="create-post-file-header-inputs">
                      <input
                        className="form-input create-post-filename-input"
                        type="text"
                        value={file.filename}
                        onChange={e => updateFile(index, 'filename', e.target.value)}
                        placeholder="filename.ts"
                        disabled={loading}
                      />
                      <input
                        className="form-input create-post-language-input"
                        type="text"
                        value={file.language}
                        onChange={e => updateFile(index, 'language', e.target.value)}
                        placeholder="language"
                        disabled={loading}
                      />
                    </div>
                    {files.length > 1 && (
                      <button
                        type="button"
                        className="create-post-file-remove"
                        onClick={() => removeFile(index)}
                        disabled={loading}
                        title="Remove file"
                      >
                        &times;
                      </button>
                    )}
                  </div>
                  <textarea
                    className="create-post-code-textarea"
                    value={file.content}
                    onChange={e => updateFile(index, 'content', e.target.value)}
                    placeholder="Paste or type code here..."
                    spellCheck={false}
                    disabled={loading}
                  />
                </div>
              ))}

              <button
                type="button"
                className="add-file-btn"
                onClick={addFile}
                disabled={loading}
              >
                + Add File
              </button>
            </div>
          </div>

          {/* Tags */}
          <div className="form-group">
            <label className="form-label">Tags</label>

            {availableTags.length > 0 && (
              <div className="create-post-tag-pills">
                {availableTags.map(tag => (
                  <button
                    key={tag.id}
                    type="button"
                    className={`create-post-tag-pill${selectedTagNames.has(tag.name) ? ' selected' : ''}`}
                    style={{ borderColor: tag.color, color: tag.color }}
                    onClick={() => toggleAdminTag(tag.name)}
                    disabled={loading}
                  >
                    {tag.name}
                  </button>
                ))}
              </div>
            )}

            {customTags.length > 0 && (
              <div className="create-post-custom-tags">
                {customTags.map(tag => (
                  <span key={tag} className="create-post-custom-tag">
                    {tag}
                    <button
                      type="button"
                      className="create-post-custom-tag-remove"
                      onClick={() => removeCustomTag(tag)}
                      disabled={loading}
                    >
                      &times;
                    </button>
                  </span>
                ))}
              </div>
            )}

            <input
              className="form-input create-post-tag-input"
              type="text"
              value={tagInput}
              onChange={e => setTagInput(e.target.value)}
              onKeyDown={handleTagInputKeyDown}
              placeholder="Add custom tag and press Enter"
              disabled={loading}
            />
            <div className="create-post-tag-hint">Press Enter to add a custom tag</div>
          </div>

          <div className="modal-actions">
            <Button type="button" variant="secondary" onClick={handleClose} disabled={loading}>
              Cancel
            </Button>
            <Button type="submit" variant="primary" loading={loading}>
              Create Post
            </Button>
          </div>
        </form>
      </div>
    </Modal>
  );
};

export default CreatePostModal;
