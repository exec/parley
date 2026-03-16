import React, { useState, useRef, useCallback, useMemo, useEffect, KeyboardEvent } from 'react';
import { Channel, Message as MessageType, ServerMember } from '../../api/types';
import { searchMessages } from '../../api/search';
import { detectMention, useMentionSuggestions, MentionSuggestion } from '../../hooks/useMentionAutocomplete';
import { detectChannelTag, useChannelSuggestions } from '../../hooks/useChannelAutocomplete';
import { MentionDropdown } from '../chat/MentionDropdown';
import { ChannelTagDropdown } from '../chat/ChannelTagDropdown';
import MarkdownRenderer from '../ui/MarkdownRenderer';
import './SearchPanel.css';

interface Props {
  serverId: string;
  members: ServerMember[];
  channels: Channel[];
  memberMap: Map<string, string>;
  channelMap: Map<string, string>;
  onClose: () => void;
  onNavigateToChannel?: (channelId: string) => void;
}

/** Parse `from:@?username` and `in:#?channelname` tokens from query text. */
function parseFilters(text: string, members: ServerMember[], channels: Channel[]) {
  const memberByUsername = new Map(members.map(m => [m.username.toLowerCase(), m.user_id]));
  const channelByName = new Map(channels.map(c => [c.name.toLowerCase(), c.id]));

  let fromUserId: string | null = null;
  let inChannelId: string | null = null;

  const fromMatch = text.match(/from:@?(\S+)/i);
  if (fromMatch) fromUserId = memberByUsername.get(fromMatch[1].toLowerCase()) ?? null;

  const inMatch = text.match(/in:#?(\S+)/i);
  if (inMatch) inChannelId = channelByName.get(inMatch[1].toLowerCase()) ?? null;

  const content = text
    .replace(/from:@?\S+/gi, '')
    .replace(/in:#?\S+/gi, '')
    .trim();

  return { content, fromUserId, inChannelId };
}

function formatTimestamp(ts: string) {
  const d = new Date(ts);
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' }) +
    ' at ' + d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
}

export const SearchPanel: React.FC<Props> = ({
  serverId,
  members,
  channels,
  memberMap,
  channelMap,
  onClose,
  onNavigateToChannel,
}) => {
  const [query, setQuery] = useState('');
  const [cursorPos, setCursorPos] = useState(0);
  const [results, setResults] = useState<MessageType[]>([]);
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);
  const [mentionSelIdx, setMentionSelIdx] = useState(0);
  const [channelSelIdx, setChannelSelIdx] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => { inputRef.current?.focus(); }, []);

  // Autocomplete detection
  const mentionMatch = useMemo(() => detectMention(query, cursorPos), [query, cursorPos]);
  const mentionSuggestions = useMentionSuggestions(mentionMatch, members);
  const channelMatch = useMemo(
    () => mentionSuggestions.length === 0 ? detectChannelTag(query, cursorPos) : null,
    [query, cursorPos, mentionSuggestions.length],
  );
  const channelSuggestions = useChannelSuggestions(channelMatch, channels);

  const handleSearch = useCallback(async () => {
    const { content, fromUserId, inChannelId } = parseFilters(query, members, channels);
    if (!content && !fromUserId && !inChannelId) return;
    setLoading(true);
    try {
      const msgs = await searchMessages(serverId, {
        q: content || undefined,
        from: fromUserId || undefined,
        in: inChannelId || undefined,
        limit: 25,
      });
      setResults(msgs);
      setSearched(true);
    } catch {
      setResults([]);
      setSearched(true);
    } finally {
      setLoading(false);
    }
  }, [query, serverId, members, channels]);

  const insertMentionToken = useCallback((suggestion: MentionSuggestion) => {
    if (!mentionMatch) return;
    const inserted = suggestion.kind === 'special' ? `${suggestion.tag} ` : `@${suggestion.member.username} `;
    const newText = query.slice(0, mentionMatch.start) + inserted + query.slice(mentionMatch.end);
    const cursor = mentionMatch.start + inserted.length;
    setQuery(newText);
    setCursorPos(cursor);
    setMentionSelIdx(0);
    requestAnimationFrame(() => {
      if (inputRef.current) {
        inputRef.current.selectionStart = cursor;
        inputRef.current.selectionEnd = cursor;
      }
    });
  }, [mentionMatch, query]);

  const insertChannelToken = useCallback((channel: Channel) => {
    if (!channelMatch) return;
    const inserted = `in:#${channel.name} `;
    const newText = query.slice(0, channelMatch.start) + inserted + query.slice(channelMatch.end);

    // If there's already an `in:` token, replace it
    const withoutExisting = newText.replace(/in:#?\S*/gi, '').trim();
    const finalText = withoutExisting + (withoutExisting ? ' ' : '') + `in:#${channel.name} `;
    const cursor = finalText.length;

    setQuery(finalText.trim() + ' ');
    setCursorPos(cursor);
    setChannelSelIdx(0);
    requestAnimationFrame(() => {
      if (inputRef.current) {
        inputRef.current.selectionStart = cursor;
        inputRef.current.selectionEnd = cursor;
      }
    });
  }, [channelMatch, query]);

  const handleKeyDown = useCallback((e: KeyboardEvent<HTMLInputElement>) => {
    if (mentionSuggestions.length > 0) {
      if (e.key === 'ArrowDown') { e.preventDefault(); setMentionSelIdx(i => (i + 1) % mentionSuggestions.length); return; }
      if (e.key === 'ArrowUp') { e.preventDefault(); setMentionSelIdx(i => (i - 1 + mentionSuggestions.length) % mentionSuggestions.length); return; }
      if (e.key === 'Tab' || e.key === 'Enter') { e.preventDefault(); insertMentionToken(mentionSuggestions[mentionSelIdx]); return; }
      if (e.key === 'Escape') { setCursorPos(query.length); return; }
    }
    if (channelSuggestions.length > 0) {
      if (e.key === 'ArrowDown') { e.preventDefault(); setChannelSelIdx(i => (i + 1) % channelSuggestions.length); return; }
      if (e.key === 'ArrowUp') { e.preventDefault(); setChannelSelIdx(i => (i - 1 + channelSuggestions.length) % channelSuggestions.length); return; }
      if (e.key === 'Tab' || e.key === 'Enter') { e.preventDefault(); insertChannelToken(channelSuggestions[channelSelIdx]); return; }
      if (e.key === 'Escape') { setCursorPos(query.length); return; }
    }
    if (e.key === 'Enter') { e.preventDefault(); handleSearch(); }
    if (e.key === 'Escape') { onClose(); }
  }, [mentionSuggestions, mentionSelIdx, insertMentionToken, channelSuggestions, channelSelIdx, insertChannelToken, handleSearch, onClose, query.length]);

  const { content: parsedContent, fromUserId, inChannelId } = useMemo(
    () => parseFilters(query, members, channels),
    [query, members, channels],
  );
  const fromName = fromUserId ? memberMap.get(fromUserId) : null;
  const inName = inChannelId ? channelMap.get(inChannelId) : null;

  return (
    <div className="search-panel">
      <div className="search-panel-header">
        <span className="search-panel-title">Search</span>
        <button className="search-panel-close" onClick={onClose} title="Close">✕</button>
      </div>

      <div className="search-panel-input-wrap" style={{ position: 'relative' }}>
        {mentionSuggestions.length > 0 && (
          <div style={{ position: 'absolute', bottom: '100%', left: 0, right: 0, zIndex: 300 }}>
            <MentionDropdown suggestions={mentionSuggestions} selectedIdx={mentionSelIdx} onSelect={insertMentionToken} />
          </div>
        )}
        {channelSuggestions.length > 0 && (
          <div style={{ position: 'absolute', bottom: '100%', left: 0, right: 0, zIndex: 300 }}>
            <ChannelTagDropdown suggestions={channelSuggestions} selectedIdx={channelSelIdx} onSelect={insertChannelToken} />
          </div>
        )}
        <input
          ref={inputRef}
          className="search-panel-input"
          value={query}
          onChange={e => { setQuery(e.target.value); setCursorPos(e.target.selectionStart ?? 0); setMentionSelIdx(0); setChannelSelIdx(0); }}
          onSelect={e => setCursorPos((e.target as HTMLInputElement).selectionStart ?? 0)}
          onClick={e => setCursorPos((e.target as HTMLInputElement).selectionStart ?? 0)}
          onKeyDown={handleKeyDown}
          placeholder='Search messages... (from:@user in:#channel)'
        />
        <button className="search-panel-btn" onClick={handleSearch} disabled={loading}>
          {loading ? '…' : '↵'}
        </button>
      </div>

      {(fromName || inName || parsedContent) && (
        <div className="search-panel-chips">
          {parsedContent && <span className="search-chip search-chip--text">"{parsedContent}"</span>}
          {fromName && <span className="search-chip search-chip--from">from: @{fromName}</span>}
          {inName && <span className="search-chip search-chip--in">in: #{inName}</span>}
        </div>
      )}

      <div className="search-panel-results">
        {loading && <div className="search-panel-status">Searching…</div>}
        {!loading && searched && results.length === 0 && (
          <div className="search-panel-status">No results found.</div>
        )}
        {!loading && results.map(msg => {
          const chName = channelMap.get(msg.channel_id) ?? msg.channel_id;
          return (
            <div key={msg.id} className="search-result">
              <div className="search-result-meta">
                <span className="search-result-channel"># {chName}</span>
                <span className="search-result-author">
                  {msg.author_display_name || msg.author_username}
                </span>
                <span className="search-result-time">{formatTimestamp(msg.created_at)}</span>
              </div>
              <div
                className="search-result-content"
                onClick={() => onNavigateToChannel?.(msg.channel_id)}
                title="Click to go to channel"
              >
                {msg.content ? (
                  <MarkdownRenderer
                    content={msg.content}
                    mode="chat"
                    memberMap={memberMap}
                    channelMap={channelMap}
                    onChannelClick={onNavigateToChannel}
                  />
                ) : (
                  msg.attachment_name && (
                    <span className="search-result-attachment">📎 {msg.attachment_name}</span>
                  )
                )}
              </div>
            </div>
          );
        })}
        {!searched && !loading && (
          <div className="search-panel-hint">
            <p>Type to search messages in this server.</p>
            <p className="search-hint-examples">
              <code>from:@username</code> — messages from a user<br />
              <code>in:#channel</code> — messages in a channel<br />
              Combine with free text for content search.
            </p>
          </div>
        )}
      </div>
    </div>
  );
};
