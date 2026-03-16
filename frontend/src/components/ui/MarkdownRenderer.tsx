import React from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { Components } from 'react-markdown';
import ShikiCodeBlock from './ShikiCodeBlock';
import './MarkdownRenderer.css';

type Mode = 'chat' | 'bio';

interface Props {
  content: string;
  mode: Mode;
  className?: string;
  /** userid → username map for rendering <@userid> mention tokens */
  memberMap?: Map<string, string>;
  /** channelid → channel name map for rendering <#channelid> tokens */
  channelMap?: Map<string, string>;
  onMiniProfile?: (userId: string, e: React.MouseEvent) => void;
  onChannelClick?: (channelId: string) => void;
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

function isSpoofed(display: string, href: string): boolean {
  if (!URL_LIKE.test(display)) return false;
  const displayHost = extractHostname(display);
  if (!displayHost) return false;
  return displayHost !== extractHostname(href);
}

const SafeLink: Components['a'] = ({ href, children, ...rest }) => {
  const childText = typeof children === 'string' ? children
    : Array.isArray(children) ? children.filter(c => typeof c === 'string').join('') : '';

  if (href && isSpoofed(childText, href)) {
    return <span className="md-spoofed-link" title={`Spoofed link blocked (actual: ${href})`}>{children}</span>;
  }

  const isExternal = href && (href.startsWith('http://') || href.startsWith('https://') || href.startsWith('//'));
  return (
    <a href={href} {...rest} {...(isExternal ? { target: '_blank', rel: 'noopener noreferrer' } : {})}>
      {children}
    </a>
  );
};

const bioHeading: (level: 1|2|3|4|5|6) => Components['h1'] = (level) =>
  function BioH({ children }) {
    const Tag = level <= 2 ? 'h4' : level <= 4 ? 'h5' : 'h6';
    return <Tag>{children}</Tag>;
  };

const BIO_COMPONENTS: Partial<Components> = {
  a: SafeLink,
  h1: bioHeading(1), h2: bioHeading(2), h3: bioHeading(3),
  h4: bioHeading(4), h5: bioHeading(5), h6: bioHeading(6),
  img: ({ alt }) => <span className="md-blocked-image">[image: {alt || 'image'}]</span>,
};

const CHAT_COMPONENTS: Partial<Components> = {
  a: SafeLink,
  code: ShikiCodeBlock,
  img: ({ src, alt }) => (
    <img src={src} alt={alt || ''} className="md-inline-image"
      style={{ maxWidth: '100%', maxHeight: 300, borderRadius: 4, display: 'block', marginTop: 4 }} />
  ),
};

// Used for inline segments (between mention tokens) — unwrap the <p> wrapper
const INLINE_COMPONENTS: Partial<Components> = {
  ...CHAT_COMPONENTS,
  p: ({ children }) => <>{children}</>,
};

const REMARK_PLUGINS = [remarkGfm];

const MENTION_RE = /(<@[^>]+>|<#[^>]+>|@everyone|@here)/;

/** Split content on mention/channel tokens and render as inline elements + pills */
function renderWithMentions(
  content: string,
  memberMap: Map<string, string>,
  wrapClass: string,
  channelMap?: Map<string, string>,
  onMiniProfile?: (userId: string, e: React.MouseEvent) => void,
  onChannelClick?: (channelId: string) => void,
): React.ReactElement {
  const segments = content.split(MENTION_RE);

  return (
    <div className={wrapClass}>
      {segments.map((seg, i) => {
        if (!seg) return null;
        // Direct user mention
        const userMatch = seg.match(/^<@([^>]+)>$/);
        if (userMatch) {
          const username = memberMap.get(userMatch[1]) ?? 'unknown';
          return (
            <span
              key={i}
              className="mention-pill mention-user"
              style={onMiniProfile ? { cursor: 'pointer' } : undefined}
              onClick={onMiniProfile ? (e) => onMiniProfile(userMatch[1], e) : undefined}
            >
              @{username}
            </span>
          );
        }
        // Channel tag
        const channelMatch = seg.match(/^<#([^>]+)>$/);
        if (channelMatch) {
          const channelName = channelMap?.get(channelMatch[1]) ?? 'unknown-channel';
          return (
            <span
              key={i}
              className="mention-pill mention-channel"
              style={onChannelClick ? { cursor: 'pointer' } : undefined}
              onClick={onChannelClick ? () => onChannelClick(channelMatch[1]) : undefined}
            >
              #{channelName}
            </span>
          );
        }
        // @everyone / @here
        if (seg === '@everyone' || seg === '@here') {
          return <span key={i} className="mention-pill mention-everyone">{seg}</span>;
        }
        // Plain text segment — render inline markdown (p unwrapped)
        return (
          <ReactMarkdown
            key={i}
            remarkPlugins={REMARK_PLUGINS}
            components={INLINE_COMPONENTS}
            disallowedElements={['html']}
            unwrapDisallowed
          >
            {seg}
          </ReactMarkdown>
        );
      })}
    </div>
  );
}

const MarkdownRenderer: React.FC<Props> = ({ content, mode, className, memberMap, channelMap, onMiniProfile, onChannelClick }) => {
  const modeClass = mode === 'chat' ? 'md-chat' : 'md-bio';
  const wrapClass = `md ${modeClass}${className ? ` ${className}` : ''}`;
  const components = mode === 'chat' ? CHAT_COMPONENTS : BIO_COMPONENTS;

  // If there are mention or channel tokens, split and render inline
  if (MENTION_RE.test(content)) {
    return renderWithMentions(content, memberMap ?? new Map(), wrapClass, channelMap, onMiniProfile, onChannelClick);
  }

  return (
    <div className={wrapClass}>
      <ReactMarkdown
        remarkPlugins={REMARK_PLUGINS}
        components={components}
        disallowedElements={['html']}
        unwrapDisallowed
      >
        {content}
      </ReactMarkdown>
    </div>
  );
};

export default MarkdownRenderer;
