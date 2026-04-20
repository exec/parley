import { useState, useEffect, type MouseEvent } from 'react';
import DOMPurify from 'dompurify';
import type { Components } from 'react-markdown';
import { highlight } from '../../lib/shiki';
import { copyToClipboard } from '../../lib/tauri';
import './CodeBlock.css';

/**
 * Custom `code` renderer for react-markdown.
 * - Fenced blocks with a language: syntax-highlighted via Shiki
 * - Inline code or no language: plain <code> element
 *
 * Safety note: dangerouslySetInnerHTML is used with Shiki output, which
 * generates sanitized HTML from parsed code tokens — not raw user HTML.
 */
const ShikiCodeBlock: Components['code'] = ({ className, children, ...props }) => {
  // react-markdown sets className to "language-<lang>" for fenced blocks
  const match = /language-(\w+)/.exec(className ?? '');
  const lang = match ? match[1] : '';
  const isBlock = Boolean(match) || (typeof children === 'string' && (children as string).includes('\n'));
  const code = String(children ?? '').replace(/\n$/, '');

  const [html, setHtml] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!lang) return;
    let cancelled = false;
    highlight(code, lang).then((result) => {
      if (!cancelled) setHtml(result);
    });
    return () => { cancelled = true; };
  }, [code, lang]);

  const handleCopy = async (e: MouseEvent) => {
    e.stopPropagation();
    try {
      await copyToClipboard(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard API unavailable; silently ignore
    }
  };

  const copyButton = (
    <button
      className={`code-block-copy${copied ? ' code-block-copy--copied' : ''}`}
      onClick={handleCopy}
      title="Copy code to clipboard"
      type="button"
    >
      {copied ? 'Copied!' : 'Copy'}
    </button>
  );

  if (!isBlock || !lang) {
    return (
      <code className={className} {...props}>
        {children}
      </code>
    );
  }

  if (html !== null) {
    const shikiHtml = html;
    return (
      <div className="code-block" style={{ margin: '8px 0' }}>
        {copyButton}
        <div className="code-block-body">
          <ShikiOutput html={shikiHtml} />
        </div>
      </div>
    );
  }

  // Fallback while loading
  return (
    <div className="code-block" style={{ margin: '8px 0' }}>
      {copyButton}
      <div className="code-block-body">
        <pre className="code-block-plain">{code}</pre>
      </div>
    </div>
  );
};

function ShikiOutput({ html }: { html: string }) {
  return <div dangerouslySetInnerHTML={{ __html: DOMPurify.sanitize(html) }} />;
}

export default ShikiCodeBlock;
