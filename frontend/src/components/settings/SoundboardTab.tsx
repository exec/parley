import React, { useEffect, useState, useRef } from 'react';
import { Trash2, Upload, Smile, X, FileAudio } from 'lucide-react';
import { Sound, listServerSounds, uploadSound, deleteSound } from '../../api/soundboard';
import { EmojiPicker } from '../chat/EmojiPicker';
import './SoundboardTab.css';

const MAX_SOUNDS = 48;
const MAX_FILE_BYTES = 5 * 1024 * 1024;

function formatBytes(n: number): string {
  if (n >= 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  if (n >= 1024) return `${(n / 1024).toFixed(0)} KB`;
  return `${n} B`;
}

interface Props {
  serverId: number;
}

export const SoundboardTab: React.FC<Props> = ({ serverId }) => {
  const [sounds, setSounds] = useState<Sound[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  // Upload form state
  const [uploadName, setUploadName] = useState('');
  const [uploadEmoji, setUploadEmoji] = useState('');
  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState('');
  const [showEmojiPicker, setShowEmojiPicker] = useState(false);
  const [pickerOpensDown, setPickerOpensDown] = useState(true);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const emojiPickerRef = useRef<HTMLDivElement>(null);
  const emojiWrapperRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setLoading(true);
    listServerSounds(String(serverId))
      .then(setSounds)
      .catch(() => setError('Failed to load sounds.'))
      .finally(() => setLoading(false));
  }, [serverId]);

  useEffect(() => {
    if (!showEmojiPicker) return;
    const handler = (e: MouseEvent) => {
      if (emojiPickerRef.current && !emojiPickerRef.current.contains(e.target as Node)) {
        setShowEmojiPicker(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [showEmojiPicker]);

  const pickFile = () => fileInputRef.current?.click();

  const onFileChange = (file: File | null) => {
    setUploadError('');
    if (file && file.size > MAX_FILE_BYTES) {
      setUploadFile(null);
      setUploadError(`File too large (${formatBytes(file.size)}). Max 5 MB.`);
      if (fileInputRef.current) fileInputRef.current.value = '';
      return;
    }
    setUploadFile(file);
    if (file && !uploadName.trim()) {
      const base = file.name.replace(/\.[^.]+$/, '').slice(0, 32);
      setUploadName(base);
    }
  };

  const clearFile = () => {
    setUploadFile(null);
    if (fileInputRef.current) fileInputRef.current.value = '';
  };

  const handleUpload = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!uploadFile || !uploadName.trim()) return;
    setUploading(true);
    setUploadError('');
    try {
      const sound = await uploadSound(String(serverId), uploadFile, uploadName.trim(), uploadEmoji.trim());
      setSounds(prev => [...prev, sound]);
      setUploadName('');
      setUploadEmoji('');
      clearFile();
    } catch (err: any) {
      setUploadError(err?.message ?? 'Upload failed.');
    } finally {
      setUploading(false);
    }
  };

  const handleDelete = async (sound: Sound) => {
    if (!window.confirm(`Delete "${sound.name}"?`)) return;
    try {
      await deleteSound(String(serverId), sound.id);
      setSounds(prev => prev.filter(s => s.id !== sound.id));
    } catch {
      alert('Failed to delete sound.');
    }
  };

  const atLimit = sounds.length >= MAX_SOUNDS;

  return (
    <div>
      <h2 className="settings-page-title">Soundboard</h2>

      <div className="settings-section">
        <div className="settings-section-title">Add Sound</div>
        <div className="settings-form-hint" style={{ marginBottom: 14 }}>
          MP3, OGG, or WAV. Up to 5 MB and 32 characters for the name. {sounds.length} / {MAX_SOUNDS} sounds used.
        </div>

        <form className="soundboard-form" onSubmit={handleUpload}>
          <div className="settings-form-group">
            <label className="settings-form-label">Audio File</label>
            <input
              ref={fileInputRef}
              type="file"
              accept=".mp3,.ogg,.wav,audio/mpeg,audio/ogg,audio/wav"
              onChange={e => onFileChange(e.target.files?.[0] ?? null)}
              className="soundboard-file-hidden"
            />
            {uploadFile ? (
              <div className="soundboard-file-chip">
                <FileAudio size={14} />
                <span className="soundboard-file-chip-name">{uploadFile.name}</span>
                <span className="soundboard-file-chip-size">{formatBytes(uploadFile.size)}</span>
                <button type="button" className="soundboard-file-chip-clear" onClick={clearFile} title="Remove">
                  <X size={14} />
                </button>
              </div>
            ) : (
              <button type="button" className="soundboard-file-pick" onClick={pickFile}>
                Choose File…
              </button>
            )}
          </div>

          <div className="settings-form-group">
            <label className="settings-form-label">Name</label>
            <input
              type="text"
              placeholder="Name (required, max 32 chars)"
              maxLength={32}
              value={uploadName}
              onChange={e => setUploadName(e.target.value)}
              className="settings-form-input"
            />
          </div>

          <div className="settings-form-group">
            <label className="settings-form-label">Emoji (optional)</label>
            <div className="soundboard-emoji-wrapper" ref={emojiWrapperRef}>
              <button
                type="button"
                className="soundboard-emoji-btn"
                onClick={() => {
                  if (!showEmojiPicker && emojiWrapperRef.current) {
                    const rect = emojiWrapperRef.current.getBoundingClientRect();
                    setPickerOpensDown(rect.bottom + 400 <= window.innerHeight - 16);
                  }
                  setShowEmojiPicker(v => !v);
                }}
                title="Pick emoji"
              >
                {uploadEmoji ? (
                  <span className="soundboard-emoji-preview">{uploadEmoji}</span>
                ) : (
                  <Smile size={16} />
                )}
              </button>
              {uploadEmoji && (
                <button
                  type="button"
                  className="soundboard-emoji-clear"
                  onClick={() => setUploadEmoji('')}
                  title="Clear emoji"
                >
                  <X size={12} />
                </button>
              )}
              {showEmojiPicker && (
                <div
                  className="soundboard-emoji-picker-anchor"
                  style={pickerOpensDown ? { top: 'calc(100% + 6px)', bottom: 'auto' } : { bottom: 'calc(100% + 6px)', top: 'auto' }}
                >
                  <EmojiPicker
                    ref={emojiPickerRef}
                    onSelect={emoji => { setUploadEmoji(emoji); setShowEmojiPicker(false); }}
                    onClose={() => setShowEmojiPicker(false)}
                  />
                </div>
              )}
            </div>
          </div>

          {uploadError && <div className="settings-error" style={{ marginBottom: 12 }}>{uploadError}</div>}
          {atLimit && (
            <div className="settings-error" style={{ marginBottom: 12 }}>
              Server has reached the {MAX_SOUNDS} sound limit.
            </div>
          )}

          <button
            type="submit"
            className="settings-save-btn"
            disabled={uploading || !uploadFile || !uploadName.trim() || atLimit}
          >
            <Upload size={14} style={{ marginRight: 6 }} />
            {uploading ? 'Uploading…' : 'Upload'}
          </button>
        </form>
      </div>

      <div className="settings-section">
        <div className="settings-section-title">Sounds</div>
        {loading ? (
          <div className="settings-form-hint">Loading…</div>
        ) : error ? (
          <div className="settings-error">{error}</div>
        ) : sounds.length === 0 ? (
          <div className="settings-form-hint">No sounds yet. Upload one above.</div>
        ) : (
          <div className="soundboard-grid">
            {sounds.map(sound => (
              <div key={sound.id} className="soundboard-card">
                <span className="soundboard-card-emoji">{sound.emoji || '🔊'}</span>
                <span className="soundboard-card-name">{sound.name}</span>
                <button
                  className="soundboard-card-delete"
                  onClick={() => handleDelete(sound)}
                  title="Delete"
                >
                  <Trash2 size={14} />
                </button>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};
