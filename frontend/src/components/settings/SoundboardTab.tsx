import React, { useEffect, useState, useRef } from 'react';
import { Trash2, Upload } from 'lucide-react';
import { Sound, listServerSounds, uploadSound, deleteSound } from '../../api/soundboard';
import './SoundboardTab.css';

const MAX_SOUNDS = 48;

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
  const fileInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    setLoading(true);
    listServerSounds(String(serverId))
      .then(setSounds)
      .catch(() => setError('Failed to load sounds.'))
      .finally(() => setLoading(false));
  }, [serverId]);

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
      setUploadFile(null);
      if (fileInputRef.current) fileInputRef.current.value = '';
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

  return (
    <div className="soundboard-tab">
      <div className="soundboard-header">
        <h3 className="soundboard-title">Soundboard</h3>
        <span className="soundboard-count">{sounds.length} / {MAX_SOUNDS} sounds</span>
      </div>

      {/* Upload form */}
      <form className="soundboard-upload-form" onSubmit={handleUpload}>
        <h4 className="soundboard-section-title">Add Sound</h4>
        <div className="soundboard-upload-row">
          <input
            ref={fileInputRef}
            type="file"
            accept=".mp3,.ogg,.wav,audio/mpeg,audio/ogg,audio/wav"
            onChange={e => setUploadFile(e.target.files?.[0] ?? null)}
            className="soundboard-file-input"
          />
          <input
            type="text"
            placeholder="Name (required, max 32 chars)"
            maxLength={32}
            value={uploadName}
            onChange={e => setUploadName(e.target.value)}
            className="soundboard-input"
          />
          <input
            type="text"
            placeholder="Emoji (optional)"
            maxLength={64}
            value={uploadEmoji}
            onChange={e => setUploadEmoji(e.target.value)}
            className="soundboard-input soundboard-input--emoji"
          />
          <button
            type="submit"
            className="soundboard-upload-btn"
            disabled={uploading || !uploadFile || !uploadName.trim() || sounds.length >= MAX_SOUNDS}
          >
            <Upload size={14} />
            {uploading ? 'Uploading\u2026' : 'Upload'}
          </button>
        </div>
        {uploadError && <p className="soundboard-error">{uploadError}</p>}
        {sounds.length >= MAX_SOUNDS && (
          <p className="soundboard-error">Server has reached the {MAX_SOUNDS} sound limit.</p>
        )}
      </form>

      {/* Sound list */}
      <div className="soundboard-section-title">Sounds</div>
      {loading ? (
        <p className="soundboard-empty">Loading\u2026</p>
      ) : error ? (
        <p className="soundboard-error">{error}</p>
      ) : sounds.length === 0 ? (
        <p className="soundboard-empty">No sounds yet. Upload one above.</p>
      ) : (
        <div className="soundboard-grid">
          {sounds.map(sound => (
            <div key={sound.id} className="soundboard-card">
              <span className="soundboard-card-emoji">{sound.emoji || '\uD83D\uDD0A'}</span>
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
  );
};
