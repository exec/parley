import { useState, useEffect } from 'react';
import type { Components } from 'react-markdown';
import { highlight } from '../../lib/shiki';
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

  useEffect(() => {
    if (!lang) return;
    let cancelled = false;
    highlight(code, lang).then((result) => {
      if (!cancelled) setHtml(result);
    });
    return () => { cancelled = true; };
  }, [code, lang]);

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
        <div className="code-block-body">
          <ShikiOutput html={shikiHtml} />
        </div>
      </div>
    );
  }

  // Fallback while loading
  return (
    <div className="code-block" style={{ margin: '8px 0' }}>
      <div className="code-block-body">
        <pre className="code-block-plain">{code}</pre>
      </div>
    </div>
  );
};

/**
 * Renders Shiki-generated HTML. Shiki produces sanitized markup from
 * parsed AST tokens — this is not user-supplied raw HTML.
 */
function ShikiOutput({ html }: { html: string }) {
  return <div dangerouslySetInnerHTML={{ __html: html }} />;
}

export default ShikiCodeBlock;
