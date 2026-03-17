// frontend/src/components/EmbedCard.tsx
import React from 'react';
import './EmbedCard.css';

interface EmbedCardProps {
  icon?: React.ReactNode;
  title: string;
  subtitle?: string;
  badge?: boolean;       // shows verified ✓ chip next to title
  preview?: React.ReactNode;
  children?: React.ReactNode;
  actions?: React.ReactNode;
}

export const EmbedCard: React.FC<EmbedCardProps> = ({
  icon, title, subtitle, badge, preview, children, actions,
}) => (
  <div className="embed-card">
    {(icon || title) && (
      <div className="embed-card-header">
        {icon && <div className="embed-card-icon">{icon}</div>}
        <div className="embed-card-title-group">
          <div className="embed-card-title">
            {title}
            {badge && <span className="embed-card-badge" title="Verified">✓</span>}
          </div>
          {subtitle && <div className="embed-card-subtitle">{subtitle}</div>}
        </div>
      </div>
    )}
    {preview && <div className="embed-card-preview">{preview}</div>}
    {children && <div className="embed-card-body">{children}</div>}
    {actions != null && <div className="embed-card-actions">{actions}</div>}
  </div>
);
