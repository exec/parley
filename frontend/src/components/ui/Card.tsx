import React from 'react';
import './styles.css';

interface CardProps {
  children: React.ReactNode;
  className?: string;
}

export const Card: React.FC<CardProps> = ({ children, className = '' }) => {
  const classes = ['card', className].filter(Boolean).join(' ');
  return <div className={classes}>{children}</div>;
};