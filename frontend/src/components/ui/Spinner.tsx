import React from 'react';
import './styles.css';

type SpinnerSize = 'sm' | 'md' | 'lg';

interface SpinnerProps {
  size?: SpinnerSize;
  className?: string;
}

export const Spinner: React.FC<SpinnerProps> = ({
  size = 'md',
  className = '',
}) => {
  const classes = ['spinner', `spinner-${size}`, className]
    .filter(Boolean)
    .join(' ');

  return <div className={classes} />;
};