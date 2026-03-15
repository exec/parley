import React, { useState } from 'react';
import { Modal } from '../ui/Modal';
import { Button } from '../ui/Button';

interface CreateChannelModalProps {
  isOpen: boolean;
  onClose: () => void;
  onCreate: (name: string, type: number, topic?: string) => Promise<void>;
}

export const CreateChannelModal: React.FC<CreateChannelModalProps> = ({ isOpen, onClose, onCreate }) => {
  const [name, setName] = useState('');
  const [type, setType] = useState(0); // 0 = text, 1 = voice
  const [topic, setTopic] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      setError('Channel name is required');
      return;
    }
    setLoading(true);
    setError('');
    try {
      await onCreate(name.trim().toLowerCase().replace(/\s+/g, '-'), type, topic.trim() || undefined);
      setName('');
      setType(0);
      setTopic('');
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create channel');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Create Channel">
      <form onSubmit={handleSubmit} className="modal-form">
        {error && <div className="modal-error">{error}</div>}
        <div className="form-group">
          <label className="form-label">Channel Type</label>
          <div className="channel-type-selector">
            <div
              className={`channel-type-option ${type === 0 ? 'selected' : ''}`}
              onClick={() => setType(0)}
            >
              <span className="channel-type-icon">#</span>
              <div>
                <div className="channel-type-name">Text Channel</div>
                <div className="channel-type-desc">Send messages, images, GIFs, and more</div>
              </div>
            </div>
            <div
              className={`channel-type-option ${type === 1 ? 'selected' : ''}`}
              onClick={() => setType(1)}
            >
              <span className="channel-type-icon">🔊</span>
              <div>
                <div className="channel-type-name">Voice Channel</div>
                <div className="channel-type-desc">Hang out together with voice</div>
              </div>
            </div>
            <div
              className={`channel-type-option ${type === 2 ? 'selected' : ''}`}
              onClick={() => setType(2)}
            >
              <span className="channel-type-icon">&lt;/&gt;</span>
              <div>
                <div className="channel-type-name">Bin Channel</div>
                <div className="channel-type-desc">Code sharing &amp; discussion</div>
              </div>
            </div>
          </div>
        </div>
        <div className="form-group">
          <label className="form-label">Channel Name</label>
          <input
            className="form-input"
            type="text"
            value={name}
            onChange={e => setName(e.target.value)}
            placeholder={type === 0 ? 'new-channel' : type === 1 ? 'General' : 'code-sharing'}
            autoFocus
          />
        </div>
        {type === 0 && (
          <div className="form-group">
            <label className="form-label">Channel Topic <span style={{ color: '#555', fontWeight: 400 }}>(optional)</span></label>
            <input
              className="form-input"
              type="text"
              value={topic}
              onChange={e => setTopic(e.target.value)}
              placeholder="What's this channel about?"
            />
          </div>
        )}
        <div className="modal-actions">
          <Button type="button" variant="secondary" onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={loading}>Create Channel</Button>
        </div>
      </form>
    </Modal>
  );
};
