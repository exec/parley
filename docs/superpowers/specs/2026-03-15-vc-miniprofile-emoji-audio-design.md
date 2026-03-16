# VC Context Menu, Mini Profile, Emoji Search Fix, Audio Compression

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan.

**Goal:** Add VC participant context menu with mute/kick, wire mini profile popup to chat usernames and mention pills, fix emoji picker search, and set audio bitrate to 24 kbps.

**Architecture:** All changes are frontend-heavy with two small backend additions (kick-from-voice endpoint + WS event). Reuses existing MiniProfile component and emoji-mart SearchIndex already in the app.

**Tech Stack:** React/TypeScript frontend, Go backend, LiveKit, @emoji-mart/data SearchIndex

---

## Feature 1: VC Participant Context Menu + Kick from Voice

### Scope

Right-clicking a non-local participant tile in the voice channel shows a context menu at cursor position. Menu items are permission-gated. Kicking sends a WS event that forces the target client to disconnect.

### Backend

**New endpoint:** `POST /channels/{channelId}/voice/participants/{targetUserId}/kick`
- Permission check: `PermKickMembers` (`int64 = 1 << 4`)
- Calls existing leave logic: removes row from voice_participants table, broadcasts `VOICE_STATE_UPDATE` (action: "leave") to server channel — reuse the same DB + broadcast logic already in the `Leave` handler
- Sends `VOICE_FORCE_DISCONNECT` WS event to target user specifically via `hub.SendToUser(targetUserID, ws.EventVoiceForceDisconnect, payload)`. Payload: `{ "channel_id": channelId }`
- Returns 204 No Content

**New WS event constant in `internal/websocket/events.go`:**
```go
EventVoiceForceDisconnect = "VOICE_FORCE_DISCONNECT"
```

**New route in `cmd/api/routes.go`:**
```go
r.Post("/channels/{channelId}/voice/participants/{targetUserId}/kick", voiceHandler.KickParticipant)
```

### Frontend

**`frontend/src/api/voice.ts`**
- Add:
  ```typescript
  export async function kickVoiceParticipant(channelId: string, targetUserId: string): Promise<void> {
    return apiClient.post<void>(`/channels/${channelId}/voice/participants/${targetUserId}/kick`, {});
  }
  ```

**`frontend/src/hooks/useWebSocket.ts`**
- Add `onVoiceForceDisconnect?: () => void` to the options interface
- Handle `VOICE_FORCE_DISCONNECT` event → call the callback (no payload inspection needed)

**`frontend/src/App.tsx`**
- Wire `onVoiceForceDisconnect` → calls `vcDisconnect()`
- `canKickMembers` already exists (line 188): `isServerOwner || (effectivePermissions & (1 | 16)) !== 0`. **Do not create a new variable.** Pass it as `canKickFromVoice={canKickMembers}` in the `<VoiceChannel>` render (alongside the existing `canMuteMembers` prop).

**`frontend/src/components/voice/VoiceChannel.tsx`**

Add to `VoiceChannelProps` interface:
```typescript
canKickFromVoice?: boolean;
```

Track context menu state:
```typescript
const [contextMenu, setContextMenu] = useState<{ participantId: string; x: number; y: number } | null>(null);
```

**Wire `onContextMenu` to participant tiles in BOTH view modes** (grid and speaker filmstrip). In both places where a non-local, non-screen-share `ParticipantTile` is rendered:
```typescript
onContextMenu={(e) => {
  e.preventDefault();
  e.stopPropagation(); // prevents filmstrip pin-on-click from firing
  setContextMenu({ participantId: p.identity, x: e.clientX, y: e.clientY });
}}
```
Screen-share tiles must NOT receive `onContextMenu`.

Add Escape key handler to close context menu:
```typescript
useEffect(() => {
  if (!contextMenu) return;
  const handleKey = (e: KeyboardEvent) => {
    if (e.key === 'Escape') setContextMenu(null);
  };
  window.addEventListener('keydown', handleKey);
  return () => window.removeEventListener('keydown', handleKey); // cleanup required
}, [contextMenu]);
```

Render `<VoiceContextMenu>` when `contextMenu !== null`:
```tsx
{contextMenu && (
  <VoiceContextMenu
    position={{ x: contextMenu.x, y: contextMenu.y }}
    participantId={contextMenu.participantId}
    canMute={!!canMuteMembers}
    canKick={!!canKickFromVoice}
    onMute={() => { onMuteParticipant?.(contextMenu.participantId); setContextMenu(null); }}
    onKick={async () => {
      try { await kickVoiceParticipant(channel.id, contextMenu.participantId); } catch (e) { console.error(e); }
      setContextMenu(null);
    }}
    onClose={() => setContextMenu(null)}
  />
)}
```

**`frontend/src/components/voice/ParticipantTile.tsx`**
- Add prop: `onContextMenu?: (e: React.MouseEvent) => void`
- Attach to the root tile div: `<div className={...} onContextMenu={onContextMenu}>`

**`frontend/src/components/voice/VoiceContextMenu.tsx`** (new file)

```tsx
import React, { useEffect, useRef } from 'react';
import './VoiceContextMenu.css';

interface VoiceContextMenuProps {
  position: { x: number; y: number };
  participantId: string;
  canMute: boolean;
  canKick: boolean;
  onMute: () => void;
  onKick: () => void;
  onClose: () => void;
}

export const VoiceContextMenu: React.FC<VoiceContextMenuProps> = ({
  position, canMute, canKick, onMute, onKick, onClose,
}) => {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [onClose]);

  const style: React.CSSProperties = {
    position: 'fixed',
    top: Math.min(position.y, window.innerHeight - 120),
    left: Math.min(position.x, window.innerWidth - 180),
    zIndex: 9999,
  };

  return (
    <div ref={ref} className="vc-context-menu" style={style}>
      {canMute && <button className="vc-context-menu-item" onClick={onMute}>Mute</button>}
      {canKick && <button className="vc-context-menu-item vc-context-menu-item--danger" onClick={onKick}>Kick from Voice</button>}
    </div>
  );
};
```

**`frontend/src/components/voice/VoiceContextMenu.css`** (new file)
```css
.vc-context-menu {
  background: #1a1a1a;
  border: 1px solid #333;
  border-radius: 6px;
  padding: 4px;
  min-width: 160px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.5);
}

.vc-context-menu-item {
  display: block;
  width: 100%;
  padding: 8px 12px;
  background: none;
  border: none;
  border-radius: 4px;
  color: #ccc;
  font-size: 14px;
  text-align: left;
  cursor: pointer;
}

.vc-context-menu-item:hover {
  background: #2a2a2a;
  color: #fff;
}

.vc-context-menu-item--danger {
  color: #cc4444;
}

.vc-context-menu-item--danger:hover {
  background: rgba(204, 68, 68, 0.15);
  color: #ff6666;
}
```

---

## Feature 2: Mini Profile Popup on Chat Username / Mention Click

### Scope

Left-clicking a username in a chat message or an @mention pill opens the existing `MiniProfile` component positioned near the cursor. The "View Profile" button inside MiniProfile opens the full modal. Right-click behavior is unchanged.

### Data

`ChatWindow` already receives `members?: ServerMember[]`. When a click occurs, look up the member by `userId` in that array. `ServerMember` is structurally compatible with `MiniProfileMember` — pass it directly.

```typescript
const member = members?.find(m => m.user_id === userId);
if (!member) return; // user not in member list — do nothing
```

No API fetch needed. No loading state needed.

### Frontend

**`frontend/src/components/chat/Message.tsx`**
- Add prop: `onMiniProfile?: (userId: string, e: React.MouseEvent) => void`
- Change left-click on avatar/username to call `onMiniProfile?.(message.author_id, e)` instead of `onViewProfile?.(...)`.
  - Note: `onMiniProfile` receives only `userId` (not username) — the username is resolved in `ChatWindow` from the member object.
- Right-click context menu still calls `onViewProfile` — **do not change right-click behavior**.

**`frontend/src/components/chat/MessageList.tsx`**
- Add to `MessageListProps` interface:
  ```typescript
  onMiniProfile?: (userId: string, e: React.MouseEvent) => void;
  ```
- Destructure `onMiniProfile` from props and thread it to each `<Message onMiniProfile={onMiniProfile} />`

**`frontend/src/components/ui/MarkdownRenderer.tsx`**
- Add prop: `onMiniProfile?: (userId: string, e: React.MouseEvent) => void`
- User mention pills get onClick:
  ```tsx
  <span
    key={i}
    className="mention-pill mention-user"
    style={{ cursor: 'pointer' }}
    onClick={(e) => onMiniProfile?.(userMatch[1], e)}
  >
    @{username}
  </span>
  ```
  Where `userMatch[1]` is the userId extracted from the `<@userId>` token.
- `@everyone` and `@here` pills remain non-clickable.

**`frontend/src/components/chat/ChatWindow.tsx`**

Add state:
```typescript
const [miniProfile, setMiniProfile] = useState<{
  member: ServerMember;
  position: { top: number; left: number };
} | null>(null);
```

Add handler:
```typescript
const handleMiniProfile = useCallback((userId: string, e: React.MouseEvent) => {
  const member = members?.find(m => m.user_id === userId);
  if (!member) return;
  const x = e.clientX;
  const y = e.clientY;
  const left = Math.min(x + 10, window.innerWidth - 290);
  const top = Math.min(y, window.innerHeight - 330);
  setMiniProfile({ member, position: { top, left } });
}, [members]);
```

Render `<MiniProfile>` when `miniProfile !== null`. Note:
- `isCurrentUser`: compare against `currentUserId` (the prop ChatWindow already receives as `currentUserId?: string`)
- `isOnline`: pass `false` — ChatWindow does not receive online status; this just controls the status dot color
- `onViewProfile`: MiniProfile calls this with one argument `(userId: string)`. Wrap it to call ChatWindow's existing `onViewProfile?.(uid, miniProfile.member.username)` then close.
- `onSendMessage`: ChatWindow's prop for DMs is `onSendMessageToUser?: (userId: string) => void`. Use that.
- `canManageRoles`: pass `false` — role management not available from chat context.

```tsx
{miniProfile && (
  <MiniProfile
    member={miniProfile.member}
    isCurrentUser={miniProfile.member.user_id === currentUserId}
    isOnline={false}
    position={miniProfile.position}
    onClose={() => setMiniProfile(null)}
    onSendMessage={(uid) => { onSendMessageToUser?.(uid); setMiniProfile(null); }}
    onViewProfile={(uid) => {
      onViewProfile?.(uid, miniProfile.member.username);
      setMiniProfile(null);
    }}
    canManageRoles={false}
  />
)}
```

Pass `onMiniProfile={handleMiniProfile}` down to `<MessageList>`.

`<MarkdownRenderer>` is already rendered inside `<Message>` which receives `onMiniProfile` — thread it through `Message` → `MarkdownRenderer` as well.

---

## Feature 3: Emoji Picker Search Fix

### Problem

`EmojiPicker.tsx` stores emojis as raw characters (`string[]`) and searches with `emoji.includes(searchTerm)`. Searching "smile" can never match "😀" because the character string doesn't contain "smile".

### Fix

Use `SearchIndex` from `emoji-mart` (already a project dependency). Note: `useEmojiAutocomplete.ts` already calls `init({ data })` via a module-level promise (`ensureInit`). Calling `init({ data })` again in `EmojiPicker.tsx` is safe — it is idempotent (multiple calls to `init` with the same data are no-ops).

**`frontend/src/components/chat/EmojiPicker.tsx`**

Add imports at the top:
```typescript
import data from '@emoji-mart/data';
import { SearchIndex, init } from 'emoji-mart';
init({ data }); // idempotent — safe to call multiple times
```

Add state and effect for async search:
```typescript
const [searchResults, setSearchResults] = useState<string[]>([]);

useEffect(() => {
  if (!search.trim()) {
    setSearchResults([]);
    return;
  }
  SearchIndex.search(search).then((results: any[]) => {
    setSearchResults(results.map((r: any) => r.skins[0].native));
  });
}, [search]);

const displayEmojis: string[] = search.trim() ? searchResults : EMOJI_CATEGORIES[activeCategory].emojis;
```

When `search` is non-empty and `searchResults` is empty (still resolving or no matches), the grid shows nothing — this is correct "no results" behavior. No changes to `EMOJI_CATEGORIES` data structure required.

---

## Feature 4: Audio Bitrate 24 kbps

**`frontend/src/hooks/useVoiceConnection.ts`**

Change the `publishTrack` call (~line 242):
```typescript
// Before:
await room.localParticipant.publishTrack(micTrack);

// After:
await room.localParticipant.publishTrack(micTrack, { audioBitrate: 24_000 });
```

LiveKit uses Opus codec by default. 24 kbps is transparent for voice and saves ~25% vs the 32 kbps default.

---

## Permissions Reference

- `PermKickMembers = int64(1 << 4)` — used for kick-from-voice; already computed in App.tsx as `canKickMembers`
- `PermMuteMembers = int64(1 << 34)` — existing, used for force-mute; already computed as `canMuteMembers`

---

## File Change Summary

| File | Change |
|------|--------|
| `internal/voice/handler.go` | Add `KickParticipant` handler |
| `internal/websocket/events.go` | Add `EventVoiceForceDisconnect` constant |
| `cmd/api/routes.go` | Register kick route |
| `frontend/src/api/voice.ts` | Add `kickVoiceParticipant` |
| `frontend/src/hooks/useWebSocket.ts` | Handle `VOICE_FORCE_DISCONNECT` event |
| `frontend/src/hooks/useVoiceConnection.ts` | `audioBitrate: 24_000` on publishTrack |
| `frontend/src/App.tsx` | Wire disconnect handler, pass `canKickFromVoice={canKickMembers}` to VoiceChannel |
| `frontend/src/components/voice/VoiceContextMenu.tsx` | New context menu component |
| `frontend/src/components/voice/VoiceContextMenu.css` | Styles for context menu |
| `frontend/src/components/voice/VoiceChannel.tsx` | Context menu state + Escape handler + render; `canKickFromVoice` prop; wire `onContextMenu` on grid AND filmstrip tiles |
| `frontend/src/components/voice/ParticipantTile.tsx` | `onContextMenu` prop |
| `frontend/src/components/chat/Message.tsx` | `onMiniProfile` prop; left-click avatar/name triggers it |
| `frontend/src/components/chat/MessageList.tsx` | Add + thread `onMiniProfile` prop |
| `frontend/src/components/ui/MarkdownRenderer.tsx` | `onMiniProfile` prop; clickable user mention pills |
| `frontend/src/components/chat/ChatWindow.tsx` | Mini profile state, `handleMiniProfile`, `<MiniProfile>` render |
| `frontend/src/components/chat/EmojiPicker.tsx` | SearchIndex-based async search |
