import { useState } from 'react';
import type { PendingUpdate } from '../lib/updater';
import './UpdateBanner.css';

interface Props {
  update: PendingUpdate;
  onDismiss: () => void;
}

export function UpdateBanner({ update, onDismiss }: Props) {
  const [installing, setInstalling] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleInstall = async () => {
    setInstalling(true);
    setError(null);
    try {
      await update.install();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Install failed');
      setInstalling(false);
    }
  };

  return (
    <div className="update-banner" role="status">
      <div className="update-banner-text">
        <strong>Parley v{update.version} is available.</strong>
        {error && <span className="update-banner-error"> {error}</span>}
      </div>
      <div className="update-banner-actions">
        <button
          className="update-banner-btn update-banner-btn-primary"
          onClick={handleInstall}
          disabled={installing}
        >
          {installing ? 'Installing…' : 'Restart to update'}
        </button>
        <button
          className="update-banner-btn"
          onClick={onDismiss}
          disabled={installing}
        >
          Later
        </button>
      </div>
    </div>
  );
}
