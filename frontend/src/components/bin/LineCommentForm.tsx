import React, { useState, useRef, useEffect } from 'react';
import { createLineComment } from '../../api/bin';
import { BinLineComment } from '../../api/types';
import './LineCommentForm.css';

interface LineCommentFormProps {
  postId: string;
  versionId: string;
  fileId: string;
  lineNumber: number;
  onCreated: (comment: BinLineComment) => void;
  onCancel: () => void;
}

export const LineCommentForm: React.FC<LineCommentFormProps> = ({
  postId,
  versionId,
  fileId,
  lineNumber,
  onCreated,
  onCancel,
}) => {
  const [content, setContent] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  const handleSubmit = async () => {
    const trimmed = content.trim();
    if (!trimmed) return;

    setSubmitting(true);
    setError(null);
    try {
      const comment = await createLineComment(postId, {
        version_id: versionId,
        file_id: fileId,
        line_number: lineNumber,
        content: trimmed,
      });
      onCreated(comment);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to post comment');
      setSubmitting(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      handleSubmit();
    }
    if (e.key === 'Escape') {
      onCancel();
    }
  };

  return (
    <div className="line-comment-form">
      <div className="line-comment-form-header">
        Comment on line {lineNumber}
      </div>
      <textarea
        ref={textareaRef}
        className="line-comment-form-textarea"
        value={content}
        onChange={(e) => setContent(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="Write a comment... (Ctrl+Enter to submit)"
        rows={3}
        disabled={submitting}
      />
      {error && <div className="line-comment-form-error">{error}</div>}
      <div className="line-comment-form-actions">
        <button
          className="line-comment-form-cancel"
          onClick={onCancel}
          disabled={submitting}
        >
          Cancel
        </button>
        <button
          className="line-comment-form-submit"
          onClick={handleSubmit}
          disabled={submitting || !content.trim()}
        >
          {submitting ? 'Posting...' : 'Submit'}
        </button>
      </div>
    </div>
  );
};

export default LineCommentForm;
