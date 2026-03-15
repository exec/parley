import React, { useState, useEffect } from 'react';
import { highlight } from '../../lib/shiki';
import './CodeBlock.css';

interface CodeBlockProps {
  content: string;
  language?: string;
  filename?: string;
  showLineNumbers?: boolean;
  collapsible?: boolean;
  defaultCollapsed?: boolean;
  highlightedLines?: Set<number>;
  onLineClick?: (lineNumber: number) => void;
}

export const CodeBlock: React.FC<CodeBlockProps> = ({
  content,
  language = '',
  filename,
  showLineNumbers = true,
  collapsible = false,
  defaultCollapsed = false,
  highlightedLines,
  onLineClick,
}) => {
  const [html, setHtml] = useState<string | null>(null);
  const [collapsed, setCollapsed] = useState(defaultCollapsed);

  const lang = language || 'plaintext';
  const displayLang = language || (filename ? '' : 'plaintext');

  useEffect(() => {
    let cancelled = false;
    setHtml(null);
    highlight(content, lang).then((result) => {
      if (!cancelled) setHtml(result);
    });
    return () => { cancelled = true; };
  }, [content, lang]);

  const lines = content.split('\n');
  // Remove trailing empty line that split adds if content ends with \n
  if (lines[lines.length - 1] === '') lines.pop();

  const header = (filename || displayLang) ? (
    <div
      className="code-block-header"
      onClick={collapsible ? () => setCollapsed(c => !c) : undefined}
    >
      {filename && <span className="code-block-filename">{filename}</span>}
      {displayLang && <span className="code-block-lang">{displayLang}</span>}
      {collapsible && (
        <button
          className="code-block-toggle"
          onClick={(e) => { e.stopPropagation(); setCollapsed(c => !c); }}
          title={collapsed ? 'Expand' : 'Collapse'}
        >
          {collapsed ? '\u25b6 expand' : '\u25bc collapse'}
        </button>
      )}
    </div>
  ) : null;

  const blockClass = [
    'code-block',
    collapsible ? 'code-block-collapsible' : '',
    collapsed ? 'code-block-collapsed' : '',
  ].filter(Boolean).join(' ');

  if (showLineNumbers) {
    return (
      <div className={blockClass}>
        {header}
        <div className="code-block-body">
          <div className="code-block-lines">
            {lines.map((line, i) => {
              const lineNum = i + 1;
              const isHighlighted = highlightedLines?.has(lineNum) ?? false;
              return (
                <div
                  key={i}
                  className={`code-block-line${isHighlighted ? ' code-block-line--highlighted' : ''}`}
                >
                  <span
                    className="code-block-line-number"
                    onClick={() => onLineClick?.(lineNum)}
                    title={`Line ${lineNum}`}
                  >
                    {lineNum}
                  </span>
                  <span className="code-block-line-content">{line}</span>
                </div>
              );
            })}
          </div>
        </div>
      </div>
    );
  }

  // Safe: Shiki generates sanitized HTML from parsed code tokens, not raw user HTML.
  // nosemgrep: react-dangerouslysetinnerhtml
  return (
    <div className={blockClass}>
      {header}
      <div className="code-block-body">
        {html !== null ? (
          // eslint-disable-next-line react/no-danger
          <div dangerouslySetInnerHTML={{ __html: html }} />
        ) : (
          <pre className="code-block-plain">{content}</pre>
        )}
      </div>
    </div>
  );
};

export default CodeBlock;
