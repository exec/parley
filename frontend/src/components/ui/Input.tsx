import React from 'react';
import './styles.css';

interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
}

export const Input: React.FC<InputProps> = ({
  label,
  error,
  type = 'text',
  placeholder,
  value,
  onChange,
  className = '',
  ...props
}) => {
  const inputClasses = ['input', error ? 'input-error' : '', className]
    .filter(Boolean)
    .join(' ');

  return (
    <div className="input-wrapper">
      {label && <label className="input-label">{label}</label>}
      <input
        type={type}
        className={inputClasses}
        placeholder={placeholder}
        value={value}
        onChange={onChange}
        {...props}
      />
      {error && <span className="input-error-message">{error}</span>}
    </div>
  );
};