import React from 'react';
import './styles.css';

type AvatarSize = 'sm' | 'md' | 'lg';

interface AvatarProps {
  src?: string;
  alt?: string;
  size?: AvatarSize;
  fallback?: string;
}

export const Avatar: React.FC<AvatarProps> = ({
  src,
  alt = '',
  size = 'md',
  fallback = '',
}) => {
  const classes = ['avatar', `avatar-${size}`].filter(Boolean).join(' ');

  const getInitials = (name: string): string => {
    return name
      .split(' ')
      .map((n) => n[0])
      .join('')
      .toUpperCase()
      .slice(0, 2);
  };

  if (src) {
    return (
      <div className={classes}>
        <img src={src} alt={alt} className="avatar-img" />
      </div>
    );
  }

  return (
    <div className={classes}>
      <span>{fallback ? getInitials(fallback) : '?'}</span>
    </div>
  );
};