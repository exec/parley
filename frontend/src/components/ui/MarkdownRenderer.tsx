import React from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { Components } from 'react-markdown';
import './MarkdownRenderer.css';

type Mode = 'chat' | 'bio';

interface Props {
  content: string;
  mode: Mode;
  className?: string;
}

// Detects display text that looks like a URL or URL-fragment.
const URL_LIKE = /(?:https?:\/\/|ftp:\/\/|www\.|[a-z0-9][\w-]*\.[a-z]{2,6}(?:[/?#\s]|$))/i;

function extractHostname(s: string): string {
  s = s.toLowerCase().trim();
  const protoIdx = s.indexOf('://');
  if (protoIdx >= 0) s = s.slice(protoIdx + 3);
  s = s.replace(/^www\./, '');
  const stop = s.search(/[/?#\s]/);
  return stop >= 0 ? s.slice(0, stop) : s;
}

/** Returns true if [display](href) looks like a spoofed link. */
function isSpoofed(display: string, href: string): boolean {
  if (!URL_LIKE.test(display)) return false;
  const displayHost = extractHostname(display);
  if (!displayHost) return false; // pure protocol like "https://"
  return displayHost !== extractHostname(href);
}

// Shared link renderer — blocks spoofed links, forces new-tab on external links.
const SafeLink: Components['a'] = ({ href, children, ...rest }) => {
  const childText = typeof children === 'string' ? children
    : Array.isArray(children) ? children.filter(c => typeof c === 'string').join('') : '';

  if (href && isSpoofed(childText, href)) {
    // Render as plain text — don't give it any link affordance
    return <span className="md-spoofed-link" title={`Spoofed link blocked (actual: ${href})`}>{children}</span>;
  }

  const isExternal = href && (href.startsWith('http://') || href.startsWith('https://') || href.startsWith('//'));
  return (
    <a
      href={href}
      {...rest}
      {...(isExternal ? { target: '_blank', rel: 'noopener noreferrer' } : {})}
    >
      {children}
    </a>
  );
};

// Bio mode: downscale all headers so they don't dominate the layout
const bioHeading: (level: 1|2|3|4|5|6) => Components['h1'] = (level) =>
  function BioH({ children }) {
    const Tag = level <= 2 ? 'h4' : level <= 4 ? 'h5' : 'h6';
    return <Tag>{children}</Tag>;
  };

const BIO_COMPONENTS: Partial<Components> = {
  a: SafeLink,
  // Remap all heading levels to smaller tags
  h1: bioHeading(1),
  h2: bioHeading(2),
  h3: bioHeading(3),
  h4: bioHeading(4),
  h5: bioHeading(5),
  h6: bioHeading(6),
  // No images in bio
  img: ({ alt }) => <span className="md-blocked-image">[image: {alt || 'image'}]</span>,
};

const CHAT_COMPONENTS: Partial<Components> = {
  a: SafeLink,
  img: ({ src, alt }) => (
    <img src={src} alt={alt || ''} className="md-inline-image" style={{ maxWidth: '100%', maxHeight: 300, borderRadius: 4, display: 'block', marginTop: 4 }} />
  ),
};

const REMARK_PLUGINS = [remarkGfm];

const MarkdownRenderer: React.FC<Props> = ({ content, mode, className }) => {
  const modeClass = mode === 'chat' ? 'md-chat' : 'md-bio';
  const components = mode === 'chat' ? CHAT_COMPONENTS : BIO_COMPONENTS;

  return (
    <div className={`md ${modeClass}${className ? ` ${className}` : ''}`}>
      <ReactMarkdown
        remarkPlugins={REMARK_PLUGINS}
        components={components}
        // Never allow raw HTML — security critical
        disallowedElements={['html']}
        unwrapDisallowed
      >
        {content}
      </ReactMarkdown>
    </div>
  );
};

export default MarkdownRenderer;
