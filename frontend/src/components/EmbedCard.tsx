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
  frostedBg?: { color: string; blur: string };
}

export const EmbedCard: React.FC<EmbedCardProps> = ({
  icon, title, subtitle, badge, preview, children, actions, frostedBg,
}) => (
  <div className="embed-card" style={frostedBg ? {
    background: frostedBg.color,
    backdropFilter: `blur(${frostedBg.blur})`,
    WebkitBackdropFilter: `blur(${frostedBg.blur})`,
    borderColor: 'rgba(255,255,255,0.12)',
  } : undefined}>
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
