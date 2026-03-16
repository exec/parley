# VC Context Menu, Mini Profile, Emoji Search Fix, Audio Compression Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add VC participant right-click context menu with mute/kick, wire mini profile popup to chat usernames and mention pills, fix emoji picker search, and set audio bitrate to 24 kbps.

**Architecture:** Backend adds one new endpoint and one WS event constant following the existing MuteParticipant pattern. Frontend adds a new VoiceContextMenu component, extends VoiceChannel/ParticipantTile for context menus, threads a new `onMiniProfile` prop through MessageList→Message→MarkdownRenderer, and replaces the broken emoji search with emoji-mart's SearchIndex (already a dependency).

**Tech Stack:** Go backend, React/TypeScript frontend, LiveKit, @emoji-mart/data SearchIndex (already imported in useEmojiAutocomplete.ts), MiniProfile component already exists at `frontend/src/components/layout/MiniProfile.tsx`

**Spec:** `docs/superpowers/specs/2026-03-15-vc-miniprofile-emoji-audio-design.md`

---

## Chunk 1: Backend + Voice Context Menu

### Task 1: Backend — EventVoiceForceDisconnect + KickParticipant handler + route

**Files:**
- Modify: `internal/websocket/events.go:44-45`
- Modify: `internal/voice/handler.go:163-215` (add after MuteParticipant)
- Modify: `cmd/api/routes.go:123`

**Context:** The existing MuteParticipant handler (lines 165–215 of handler.go) is the pattern to follow exactly. KickParticipant differs in three ways: (1) it uses `PermKickMembers` instead of `PermMuteMembers`, (2) it calls `h.svc.Leave()` + `h.broadcastVoiceState()` to remove the user, (3) it sends `VOICE_FORCE_DISCONNECT` instead of `VOICE_FORCE_MUTE`. The `broadcastVoiceState` helper needs username + avatarURL for the leave broadcast — fetch the target member from DB the same way MuteParticipant fetches the channel.

- [ ] **Step 1: Add EventVoiceForceDisconnect constant**

In `internal/websocket/events.go`, add after line 45 (`EventVoiceForceMute`):
```go
EventVoiceForceDisconnect = "VOICE_FORCE_DISCONNECT"
```

- [ ] **Step 2: Add KickParticipant handler**

In `internal/voice/handler.go`, add this method after `MuteParticipant` (after line 215):
```go
// KickParticipant force-disconnects a participant from a voice channel.
// POST /channels/{channelId}/voice/participants/{targetUserId}/kick
func (h *Handler) KickParticipant(w http.ResponseWriter, r *http.Request) {
	requesterIDStr := auth.GetUserIDFromContext(r)
	requesterID, err := strconv.ParseInt(requesterIDStr, 10, 64)
	if err != nil {
		jsonErr(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	channelIDStr := r.PathValue("channelId")
	channelID, err := strconv.ParseInt(channelIDStr, 10, 64)
	if err != nil {
		jsonErr(w, "invalid channel id", http.StatusBadRequest)
		return
	}

	targetUserIDStr := r.PathValue("targetUserId")
	targetUserID, err := strconv.ParseInt(targetUserIDStr, 10, 64)
	if err != nil {
		jsonErr(w, "invalid target user id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	ch, err := h.repo.GetChannelByID(ctx, channelID)
	if err != nil {
		jsonErr(w, "channel not found", http.StatusNotFound)
		return
	}

	srv, err := h.repo.GetServerByID(ctx, ch.ServerID)
	if err != nil {
		jsonErr(w, "server not found", http.StatusNotFound)
		return
	}

	ok, err := permissions.HasChannelPermission(ctx, h.repo, ch.ServerID, requesterID, srv.OwnerID, channelID, permissions.PermKickMembers)
	if err != nil || !ok {
		jsonErr(w, "forbidden", http.StatusForbidden)
		return
	}

	// Look up target member for the broadcast payload
	targetMember, err := h.repo.GetMember(ctx, ch.ServerID, targetUserID)
	if err != nil || targetMember == nil {
		jsonErr(w, "target member not found", http.StatusNotFound)
		return
	}

	serverVirtualChannelID := "server:" + strconv.FormatInt(ch.ServerID, 10)

	// Remove from DB and broadcast leave state
	if err := h.svc.Leave(ctx, channelIDStr, targetUserIDStr); err != nil {
		jsonErr(w, "failed to remove participant", http.StatusInternalServerError)
		return
	}
	h.broadcastVoiceState(serverVirtualChannelID, channelIDStr, targetUserIDStr, targetMember.Username, targetMember.AvatarURL, "leave")

	// Send disconnect event to the target user
	payload, _ := json.Marshal(map[string]interface{}{
		"channel_id": channelIDStr,
	})
	if err := h.hub.SendToUser(targetUserIDStr, ws.EventVoiceForceDisconnect, payload); err != nil {
		// Non-fatal: user may have already left; state is already cleaned up
		_ = err
	}

	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 3: Register the route**

In `cmd/api/routes.go`, add after line 123 (the mute route):
```go
r.Post("/channels/{channelId}/voice/participants/{targetUserId}/kick", voiceHandler.KickParticipant)
```

- [ ] **Step 4: Verify it compiles**

```bash
cd /home/dylan/Developer/parley && go build ./...
```
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/websocket/events.go internal/voice/handler.go cmd/api/routes.go
git commit -m "feat: add VOICE_FORCE_DISCONNECT event and KickParticipant handler"
```

---

### Task 2: Frontend — API + useWebSocket + App.tsx wiring for kick

**Files:**
- Modify: `frontend/src/api/voice.ts:30-32` (after muteVoiceParticipant)
- Modify: `frontend/src/hooks/useWebSocket.ts:89-100` (options interface + handler + refs)
- Modify: `frontend/src/App.tsx` (import + handler + useWebSocket call + VoiceChannel prop)

**Context:** Follow the exact same pattern as `muteVoiceParticipant` in voice.ts and `onVoiceForceMute`/`handleVoiceForceMute` in useWebSocket.ts / App.tsx. The disconnect handler simply calls `vcDisconnect()` (already destructured from `useVoiceConnection` at line 154). `canKickMembers` is already computed at line 188 of App.tsx — just pass it as `canKickFromVoice` to VoiceChannel.

- [ ] **Step 1: Add kickVoiceParticipant to voice.ts**

In `frontend/src/api/voice.ts`, add after `muteVoiceParticipant` (after line 32):
```typescript
export async function kickVoiceParticipant(channelId: string, targetUserId: string): Promise<void> {
  return apiClient.post<void>(`/channels/${channelId}/voice/participants/${targetUserId}/kick`, {});
}
```

- [ ] **Step 2: Add onVoiceForceDisconnect to useWebSocket options interface**

In `frontend/src/hooks/useWebSocket.ts`, add after line 89 (`onVoiceForceMute?`):
```typescript
onVoiceForceDisconnect?: () => void;
```

- [ ] **Step 3: Add ref and effect for onVoiceForceDisconnect**

In `frontend/src/hooks/useWebSocket.ts`, after line 130 (`const onVoiceForceMuteRef = ...`):
```typescript
const onVoiceForceDisconnectRef = useRef(onVoiceForceDisconnect);
```

After line 158 (`useEffect(() => { onVoiceForceMuteRef... })`):
```typescript
useEffect(() => { onVoiceForceDisconnectRef.current = onVoiceForceDisconnect; }, [onVoiceForceDisconnect]);
```

- [ ] **Step 4: Add destructuring and event handler in useWebSocket function signature**

The function signature on line 100 is one long destructured line. Add `onVoiceForceDisconnect` alongside `onVoiceForceMute` in the destructure.

- [ ] **Step 5: Add VOICE_FORCE_DISCONNECT message handler**

In `frontend/src/hooks/useWebSocket.ts`, find the `VOICE_FORCE_MUTE` handling (lines 297-300) and add after it:
```typescript
} else if (wsMsg.type === 'VOICE_FORCE_DISCONNECT') {
  if (onVoiceForceDisconnectRef.current) {
    onVoiceForceDisconnectRef.current();
  }
```

- [ ] **Step 6: Add handler + wiring in App.tsx**

In `frontend/src/App.tsx`, add after `handleVoiceForceMute` (after line 325):
```typescript
const handleVoiceForceDisconnect = useCallback(() => {
  vcDisconnect();
}, [vcDisconnect]);
```

In the `useWebSocket` call (line 485+), add after `onVoiceForceMute: handleVoiceForceMute,` (line 507):
```typescript
onVoiceForceDisconnect: handleVoiceForceDisconnect,
```

In the `<VoiceChannel>` render (around line 676), add after `canMuteMembers={canMuteMembers}`:
```tsx
canKickFromVoice={canKickMembers}
onKickParticipant={async (userId) => {
  try { await kickVoiceParticipant(activeVoiceChannel!, userId); } catch(e) { console.error(e); }
}}
```

Add `kickVoiceParticipant` to the import from `'./api/voice'` at the top of App.tsx.

- [ ] **Step 7: Verify build**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build 2>&1 | tail -20
```
Expected: no TypeScript errors.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/api/voice.ts frontend/src/hooks/useWebSocket.ts frontend/src/App.tsx
git commit -m "feat: wire VOICE_FORCE_DISCONNECT + canKickFromVoice to VoiceChannel"
```

---

### Task 3: Frontend — VoiceContextMenu component

**Files:**
- Create: `frontend/src/components/voice/VoiceContextMenu.tsx`
- Create: `frontend/src/components/voice/VoiceContextMenu.css`

**Context:** This is a fixed-position popup rendered at cursor coordinates. It closes on outside mousedown or when Escape is pressed (Escape handled in VoiceChannel.tsx's useEffect, not here). Position is clamped to keep it within viewport: min width 180px, min height ~80px (two buttons at ~40px each).

- [ ] **Step 1: Create VoiceContextMenu.tsx**

```tsx
// frontend/src/components/voice/VoiceContextMenu.tsx
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
    top: Math.min(position.y, window.innerHeight - 100),
    left: Math.min(position.x, window.innerWidth - 190),
    zIndex: 9999,
  };

  return (
    <div ref={ref} className="vc-context-menu" style={style}>
      {canMute && (
        <button className="vc-context-menu-item" onClick={onMute}>Mute</button>
      )}
      {canKick && (
        <button className="vc-context-menu-item vc-context-menu-item--danger" onClick={onKick}>
          Kick from Voice
        </button>
      )}
    </div>
  );
};
```

- [ ] **Step 2: Create VoiceContextMenu.css**

```css
/* frontend/src/components/voice/VoiceContextMenu.css */
.vc-context-menu {
  background: #1a1a1a;
  border: 1px solid #333;
  border-radius: 6px;
  padding: 4px;
  min-width: 170px;
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

- [ ] **Step 3: Verify build**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build 2>&1 | tail -10
```
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/voice/VoiceContextMenu.tsx frontend/src/components/voice/VoiceContextMenu.css
git commit -m "feat: add VoiceContextMenu component for VC participant actions"
```

---

### Task 4: Frontend — Wire context menu into VoiceChannel + ParticipantTile

**Files:**
- Modify: `frontend/src/components/voice/VoiceChannel.tsx`
- Modify: `frontend/src/components/voice/ParticipantTile.tsx`

**Context:** VoiceChannel.tsx renders participant tiles in TWO places: (1) the grid view (lines 151-173), and (2) the filmstrip in speaker view (lines 202-229). Both need `onContextMenu` wired. Screen-share tiles (lines 136-149 in grid, the spotlight in speaker view) must NOT get a context menu. The filmstrip tile's outer `<div>` already has an `onClick` for pin — use `e.stopPropagation()` on the context menu open to prevent pin from triggering.

The new props to add to VoiceChannelProps:
- `canKickFromVoice?: boolean`
- `onKickParticipant?: (userId: string) => void`

- [ ] **Step 1: Update VoiceChannelProps interface**

In `frontend/src/components/voice/VoiceChannel.tsx`, add to the `VoiceChannelProps` interface (after line 30, after `onMuteParticipant`):
```typescript
canKickFromVoice?: boolean;
onKickParticipant?: (userId: string) => void;
```

- [ ] **Step 2: Destructure new props**

In the component function (after line 56, after `onMuteParticipant`):
```typescript
canKickFromVoice,
onKickParticipant,
```

- [ ] **Step 3: Add context menu state and Escape handler**

After the existing `useState` declarations (after line 59, after `setPinnedIdentity`):
```typescript
const [contextMenu, setContextMenu] = useState<{ participantId: string; x: number; y: number } | null>(null);

useEffect(() => {
  if (!contextMenu) return;
  const handleKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setContextMenu(null); };
  window.addEventListener('keydown', handleKey);
  return () => window.removeEventListener('keydown', handleKey);
}, [contextMenu]);
```

- [ ] **Step 4: Add import for VoiceContextMenu**

At the top of VoiceChannel.tsx, add:
```typescript
import { VoiceContextMenu } from './VoiceContextMenu';
import { kickVoiceParticipant } from '../../api/voice';
```

Wait — `onKickParticipant` is passed as a prop from App.tsx (which calls `kickVoiceParticipant`), so VoiceChannel does NOT need to import `kickVoiceParticipant` directly. Just import `VoiceContextMenu`.

- [ ] **Step 5: Update grid view participant tiles to use onContextMenu**

In the grid view (lines 151-173), change the `<div key={participant.identity} style={{ position: 'relative' }}>` wrapper to:
```tsx
<div
  key={participant.identity}
  style={{ position: 'relative' }}
  onContextMenu={!isLocal ? (e) => {
    e.preventDefault();
    e.stopPropagation();
    setContextMenu({ participantId: participant.identity, x: e.clientX, y: e.clientY });
  } : undefined}
>
```

- [ ] **Step 6: Update filmstrip tiles to use onContextMenu**

In the filmstrip (lines 205-228), the outer `<div className="vc-filmstrip-tile">` has an `onClick` for pin. Update it:
```tsx
<div
  key={participant.identity}
  className="vc-filmstrip-tile"
  style={{ position: 'relative' }}
  onClick={() => setPinnedIdentity(participant.identity)}
  onContextMenu={!isLocal ? (e) => {
    e.preventDefault();
    e.stopPropagation(); // prevents the pin onClick from firing
    setContextMenu({ participantId: participant.identity, x: e.clientX, y: e.clientY });
  } : undefined}
>
```

- [ ] **Step 7: Render VoiceContextMenu**

In the JSX return, after the closing `</div>` of the main area (before the `{/* In-channel controls */}` comment, around line 234), add:
```tsx
{contextMenu && (canMuteMembers || canKickFromVoice) && (
  <VoiceContextMenu
    position={{ x: contextMenu.x, y: contextMenu.y }}
    participantId={contextMenu.participantId}
    canMute={!!canMuteMembers}
    canKick={!!canKickFromVoice}
    onMute={() => {
      onMuteParticipant?.(contextMenu.participantId);
      setContextMenu(null);
    }}
    onKick={() => {
      onKickParticipant?.(contextMenu.participantId);
      setContextMenu(null);
    }}
    onClose={() => setContextMenu(null)}
  />
)}
```

- [ ] **Step 8: Verify build**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build 2>&1 | tail -20
```
Expected: no errors.

- [ ] **Step 9: Commit**

```bash
git add frontend/src/components/voice/VoiceChannel.tsx
git commit -m "feat: wire VC participant context menu with mute and kick actions"
```

---

## Chunk 2: Mini Profile, Emoji Search, Audio Bitrate

### Task 5: Frontend — Mini profile popup on chat username / mention click

**Files:**
- Modify: `frontend/src/components/chat/Message.tsx:15-29` (props) and `~line 298, 316` (click handlers)
- Modify: `frontend/src/components/chat/MessageList.tsx:6-22` (props) and `~line 182` (pass-through)
- Modify: `frontend/src/components/ui/MarkdownRenderer.tsx:10-16` (props) and `~line 99-101` (mention pill onClick)
- Modify: `frontend/src/components/chat/ChatWindow.tsx` (state, handler, MiniProfile render)

**Context:** The existing MiniProfile component at `frontend/src/components/layout/MiniProfile.tsx` is already fully built and used by UserSidebar. It takes `member: MiniProfileMember` — `ServerMember` is structurally compatible (superset of fields). `ChatWindow` already receives `members?: ServerMember[]` and `currentUserId?: string` and `onSendMessageToUser?: (userId: string) => void` and `onViewProfile?: (userId: string, username: string) => void`. The member lookup is instant (no fetch): `members?.find(m => m.user_id === userId)`.

Left-click on username in Message.tsx currently calls `onViewProfile?.(message.author_id, message.author_username)` on lines 298 and 316. We change these to call `onMiniProfile?.(message.author_id, e)`. Right-click context menu on lines 461-493 still calls `handleViewProfile` → unchanged.

- [ ] **Step 1: Add onMiniProfile to MessageProps**

In `frontend/src/components/chat/Message.tsx`, add to `MessageProps` interface (after line 25, after `onViewProfile`):
```typescript
onMiniProfile?: (userId: string, e: React.MouseEvent) => void;
```

- [ ] **Step 2: Destructure onMiniProfile in Message**

In the component function (around line 133, after `onViewProfile`):
```typescript
onMiniProfile,
```

- [ ] **Step 3: Update left-click handlers in Message**

On line 298, change:
```tsx
onClick={() => onViewProfile?.(message.author_id, message.author_username)}
```
to:
```tsx
onClick={(e) => onMiniProfile?.(message.author_id, e)}
```

On line 316, change the same pattern:
```tsx
onClick={(e) => onMiniProfile?.(message.author_id, e)}
```

Right-click handlers (lines ~299, 317 `onContextMenu={handleUsernameContextMenu}`) remain unchanged.

- [ ] **Step 4: Add onMiniProfile to MessageListProps**

In `frontend/src/components/chat/MessageList.tsx`, add to `MessageListProps` interface (after line 15, after `onViewProfile`):
```typescript
onMiniProfile?: (userId: string, e: React.MouseEvent) => void;
```

Destructure it (after line 33, after `onViewProfile`):
```typescript
onMiniProfile,
```

Pass to `<Message>` (near line 182, alongside `onViewProfile`):
```tsx
onMiniProfile={onMiniProfile}
```

- [ ] **Step 5: Add onMiniProfile to MarkdownRenderer props**

In `frontend/src/components/ui/MarkdownRenderer.tsx`, add to `Props` interface (after line 15, after `memberMap`):
```typescript
onMiniProfile?: (userId: string, e: React.MouseEvent) => void;
```

Destructure in the component (line 124):
```typescript
const MarkdownRenderer: React.FC<Props> = ({ content, mode, className, memberMap, onMiniProfile }) => {
```

Pass to `renderWithMentions`:
```typescript
return renderWithMentions(content, memberMap, wrapClass, onMiniProfile);
```

And update `renderWithMentions` signature:
```typescript
function renderWithMentions(
  content: string,
  memberMap: Map<string, string>,
  wrapClass: string,
  onMiniProfile?: (userId: string, e: React.MouseEvent) => void,
): React.ReactElement {
```

Update the user mention pill (lines 99-101) to:
```tsx
if (userMatch) {
  const userId = userMatch[1];
  const username = memberMap.get(userId) ?? 'unknown';
  return (
    <span
      key={i}
      className="mention-pill mention-user"
      style={onMiniProfile ? { cursor: 'pointer' } : undefined}
      onClick={onMiniProfile ? (e) => onMiniProfile(userId, e) : undefined}
    >
      @{username}
    </span>
  );
}
```

- [ ] **Step 6: Thread onMiniProfile through Message → MarkdownRenderer**

In `frontend/src/components/chat/Message.tsx`, find where `<MarkdownRenderer>` is rendered (search for `<MarkdownRenderer`). Pass `onMiniProfile`:
```tsx
<MarkdownRenderer
  content={message.content}
  mode="chat"
  memberMap={memberMap}
  onMiniProfile={onMiniProfile}
/>
```

- [ ] **Step 7: Add mini profile state + handler + render to ChatWindow**

In `frontend/src/components/chat/ChatWindow.tsx`:

Add import for MiniProfile at the top:
```typescript
import MiniProfile from '../layout/MiniProfile';
import { ServerMember } from '../../api/types';
```
(ServerMember is already imported on line 2 — just add MiniProfile import)

Add state after existing `useState` declarations (after line 75):
```typescript
const [miniProfile, setMiniProfile] = useState<{
  member: ServerMember;
  position: { top: number; left: number };
} | null>(null);
```

Add handler (after `handleSendMessage` useCallback, around line 109):
```typescript
const handleMiniProfile = useCallback((userId: string, e: React.MouseEvent) => {
  const member = members?.find(m => m.user_id === userId);
  if (!member) return;
  const left = Math.min(e.clientX + 10, window.innerWidth - 290);
  const top = Math.min(e.clientY, window.innerHeight - 330);
  setMiniProfile({ member, position: { top, left } });
}, [members]);
```

Pass `onMiniProfile` to `<MessageList>` (alongside `onViewProfile` around line 157):
```tsx
onMiniProfile={handleMiniProfile}
```

Render `<MiniProfile>` before the closing `</div>` of the chat-window (before line 177):
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

- [ ] **Step 8: Verify build**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build 2>&1 | tail -20
```
Expected: no TypeScript errors.

- [ ] **Step 9: Commit**

```bash
git add frontend/src/components/chat/Message.tsx frontend/src/components/chat/MessageList.tsx frontend/src/components/ui/MarkdownRenderer.tsx frontend/src/components/chat/ChatWindow.tsx
git commit -m "feat: mini profile popup on chat username and mention pill click"
```

---

### Task 6: Frontend — Fix emoji picker search

**Files:**
- Modify: `frontend/src/components/chat/EmojiPicker.tsx`

**Context:** The current search on line 56 does `e.includes(search)` on the raw emoji character — can never match text. Fix: use `SearchIndex` from `emoji-mart` (already a dependency used in `useEmojiAutocomplete.ts`). `SearchIndex.search(term)` returns a Promise of emoji objects; each has `.skins[0].native` for the character. Calling `init({ data })` at module level is safe — it's idempotent (the existing init in useEmojiAutocomplete.ts uses a promise guard, but calling init again is a no-op).

The component needs `useEffect` (add to imports) to trigger search asynchronously.

- [ ] **Step 1: Update EmojiPicker.tsx**

Replace the entire file:
```tsx
import React, { useState, useEffect } from 'react';
import data from '@emoji-mart/data';
import { SearchIndex, init } from 'emoji-mart';

init({ data }); // idempotent — safe alongside useEmojiAutocomplete.ts's init

const EMOJI_CATEGORIES = [
  {
    name: 'Smileys',
    icon: '😊',
    emojis: ['😀','😃','😄','😁','😆','😅','🤣','😂','🙂','🙃','😉','😊','😇','🥰','😍','🤩','😘','😗','😚','😙','🥲','😋','😛','😜','🤪','😝','🤑','🤗','🤭','🤫','🤔','🤐','🤨','😐','😑','😶','😏','😒','🙄','😬','🤥','😌','😔','😪','🤤','😴','😷','🤒','🤕','🤢','🤮','🤧','🥵','🥶','🥴','😵','💫','🤯','🤠','🥳','🥸','😎','🤓','🧐','😕','😟','🙁','☹️','😮','😯','😲','😳','🥺','😦','😧','😨','😰','😥','😢','😭','😱','😖','😣','😞','😓','😩','😫','🥱','😤','😡','😠','🤬','😈','👿','💀','☠️','💩','🤡','👹','👺','👻','👽','👾','🤖'],
  },
  {
    name: 'People',
    icon: '👋',
    emojis: ['👋','🤚','🖐️','✋','🖖','👌','🤌','🤏','✌️','🤞','🤟','🤘','🤙','👈','👉','👆','🖕','👇','☝️','👍','👎','✊','👊','🤛','🤜','👏','🙌','🫶','👐','🤲','🤝','🙏','✍️','💅','🤳','💪','🦾','🦿','🦵','🦶','👂','🦻','👃','👀','👁️','👅','🦷','🫀','🫁','🧠','🦴','💋','💌','💘','💝','💖','💗','💓','💞','💕','💟','❣️','💔','❤️','🧡','💛','💚','💙','💜','🤎','🖤','🤍'],
  },
  {
    name: 'Animals',
    icon: '🐶',
    emojis: ['🐶','🐱','🐭','🐹','🐰','🦊','🐻','🐼','🐨','🐯','🦁','🐮','🐷','🐸','🐵','🙈','🙉','🙊','🐔','🐧','🐦','🐤','🦆','🦅','🦉','🦇','🐺','🐗','🐴','🦄','🐝','🐛','🦋','🐌','🐞','🐜','🦟','🦗','🕷️','🦂','🐢','🐍','🦎','🦖','🦕','🐙','🦑','🦐','🦞','🦀','🐡','🐠','🐟','🐬','🐳','🐋','🦈','🐊','🐅','🐆','🦓','🦍','🦧','🐘','🦛','🦏','🐪','🐫','🦒','🦘','🐃','🐂','🐄','🐎','🐖','🐏','🐑','🦙','🐐','🦌','🐕','🐩','🦮','🐕‍🦺','🐈','🐈‍⬛','🪶','🐓','🦃','🦤','🦚','🦜','🦢','🦩','🕊️','🐇','🦝','🦨','🦡','🦦','🦥','🐁','🐀','🐿️','🦔'],
  },
  {
    name: 'Food',
    icon: '🍕',
    emojis: ['🍎','🍐','🍊','🍋','🍌','🍉','🍇','🍓','🫐','🍈','🍒','🍑','🥭','🍍','🥥','🥝','🍅','🍆','🥑','🥦','🥬','🥒','🌶️','🫑','🧄','🧅','🥔','🍠','🥐','🥯','🍞','🥖','🥨','🧀','🥚','🍳','🧈','🥞','🧇','🥓','🥩','🍗','🍖','🌭','🍔','🍟','🍕','🫓','🥪','🥙','🧆','🌮','🌯','🫔','🥗','🥘','🫕','🥫','🍱','🍘','🍙','🍚','🍛','🍜','🍝','🍠','🍢','🍣','🍤','🍥','🥮','🍡','🥟','🥠','🥡','🍦','🍧','🍨','🍩','🍪','🎂','🍰','🧁','🥧','🍫','🍬','🍭','🍮','🍯','🍼','🥛','☕','🫖','🍵','🧃','🥤','🧋','🍶','🍺','🍻','🥂','🍷','🥃','🍸','🍹','🧉','🍾'],
  },
  {
    name: 'Activities',
    icon: '⚽',
    emojis: ['⚽','🏀','🏈','⚾','🥎','🎾','🏐','🏉','🥏','🎱','🏓','🏸','🏒','🏑','🥍','🏏','🪃','🥅','⛳','🪁','🏹','🎣','🤿','🥊','🥋','🎽','🛹','🛼','🛷','⛸️','🥌','🎿','⛷️','🏂','🪂','🏋️','🤼','🤸','⛹️','🤺','🏇','🧘','🏄','🏊','🤽','🚣','🧗','🚵','🚴','🏆','🥇','🥈','🥉','🏅','🎖️','🏵️','🎗️','🎫','🎟️','🎪','🤹','🎭','🩰','🎨','🎬','🎤','🎧','🎼','🎵','🎶','🥁','🪘','🎷','🎺','🪗','🎸','🪕','🎻','🎲','♟️','🎯','🎳','🎮','🎰','🧩'],
  },
  {
    name: 'Travel',
    icon: '✈️',
    emojis: ['🚗','🚕','🚙','🚌','🚎','🏎️','🚓','🚑','🚒','🚐','🛻','🚚','🚛','🚜','🦯','🦽','🦼','🛴','🚲','🛵','🏍️','🛺','🚨','🚥','🚦','🛑','🚧','⚓','🛟','⛵','🚤','🛥️','🛳️','⛴️','🚢','✈️','🛩️','🛫','🛬','🪂','💺','🚁','🚟','🚠','🚡','🛰️','🚀','🛸','🎆','🎇','🗺️','🗾','🏔️','⛰️','🌋','🗻','🏕️','🏖️','🏜️','🏝️','🏞️','🏟️','🏛️','🏗️','🧱','🪨','🪵','🛖','🏘️','🏚️','🏠','🏡','🏢','🏣','🏤','🏥','🏦','🏨','🏩','🏪','🏫','🏬','🏭','🏯','🏰','💒','🗼','🗽','⛪','🕌','🛕','🕍','⛩️','🕋'],
  },
  {
    name: 'Objects',
    icon: '💡',
    emojis: ['⌚','📱','📲','💻','⌨️','🖥️','🖨️','🖱️','🖲️','🕹️','🗜️','💽','💾','💿','📀','📼','📷','📸','📹','🎥','📽️','🎞️','📞','☎️','📟','📠','📺','📻','🧭','⏱️','⏲️','⏰','🕰️','⌛','⏳','📡','🔋','🪫','🔌','💡','🔦','🕯️','🪔','🧯','🛢️','💰','💴','💵','💶','💷','💸','💳','🪙','💹','✉️','📧','📨','📩','📤','📥','📦','📫','📪','📬','📭','📮','🗳️','✏️','✒️','🖋️','🖊️','📝','📁','📂','🗂️','📅','📆','🗒️','🗓️','📇','📈','📉','📊','📋','📌','📍','🗺️','📎','🖇️','📏','📐','✂️','🗃️','🗄️','🗑️','🔒','🔓','🔏','🔐','🔑','🗝️','🔨','🪓','⛏️','⚒️','🛠️','🗡️','⚔️','🛡️','🪚','🔧','🪛','🔩','⚙️','🗜️','🔗','⛓️','🧰','🧲','🪜'],
  },
  {
    name: 'Symbols',
    icon: '❤️',
    emojis: ['❤️','🧡','💛','💚','💙','💜','🖤','🤍','🤎','💔','❤️‍🔥','❤️‍🩹','❣️','💕','💞','💓','💗','💖','💘','💝','💟','☮️','✝️','☪️','🕉️','☸️','✡️','🔯','🕎','☯️','☦️','🛐','⛎','♈','♉','♊','♋','♌','♍','♎','♏','♐','♑','♒','♓','🆔','⚛️','🉑','☢️','☣️','📴','📳','🈶','🈚','🈸','🈺','🈷️','✴️','🆚','💮','🉐','㊙️','㊗️','🈴','🈵','🈹','🈲','🅰️','🅱️','🆎','🆑','🅾️','🆘','❌','⭕','🛑','⛔','📛','🚫','💯','💢','♨️','🚷','🚯','🚳','🚱','🔞','📵','🔕','🔇','🔈','🔉','🔊','📢','📣','📯','🔔','🔕','🎵','🎶','⚠️','🚸','🔱','⚜️','🔰','♻️','✅','🈯','💹','❇️','✳️','❎','🌐','💠','Ⓜ️','🌀','💤','🏧','🚾','♿','🅿️','🛗','🈳','🈹','🚺','🚹','🚼','⚧️','🚻','🚮','🎦','📶','🈁','🔣','ℹ️','🔤','🔡','🔠','🆖','🆗','🆙','🆒','🆕','🆓','0️⃣','1️⃣','2️⃣','3️⃣','4️⃣','5️⃣','6️⃣','7️⃣','8️⃣','9️⃣','🔟','🔢','#️⃣','*️⃣','▶️','⏸️','⏹️','⏺️','⏭️','⏮️','⏩','⏪','⏫','⏬','◀️','🔼','🔽','➡️','⬅️','⬆️','⬇️','↗️','↘️','↙️','↖️','↕️','↔️','↪️','↩️','⤴️','⤵️','🔀','🔁','🔂','🔄','🔃','🎵','🎶','➕','➖','➗','✖️','💲','💱','™️','©️','®️','〰️','➰','➿','🔚','🔙','🔛','🔜','🔝','✔️','☑️','🔘','🔲','🔳','▪️','▫️','◾','◽','◼️','◻️','🟥','🟧','🟨','🟩','🟦','🟪','⬛','⬜','🔶','🔷','🔸','🔹','🔺','🔻','💠','🔘','🔗'],
  },
];

interface EmojiPickerProps {
  onSelect: (emoji: string) => void;
  onClose: () => void;
}

export const EmojiPicker: React.FC<EmojiPickerProps> = ({ onSelect }) => {
  const [activeCategory, setActiveCategory] = useState(0);
  const [search, setSearch] = useState('');
  const [searchResults, setSearchResults] = useState<string[]>([]);

  useEffect(() => {
    if (!search.trim()) {
      setSearchResults([]);
      return;
    }
    (SearchIndex.search(search) as Promise<any[]>).then(results => {
      setSearchResults(results.map((r: any) => r.skins[0].native));
    });
  }, [search]);

  const displayEmojis: string[] = search.trim()
    ? searchResults
    : EMOJI_CATEGORIES[activeCategory].emojis;

  return (
    <div className="emoji-picker-full" onClick={e => e.stopPropagation()}>
      <div className="emoji-picker-search">
        <input
          autoFocus
          type="text"
          placeholder="Search emoji..."
          value={search}
          onChange={e => setSearch(e.target.value)}
          className="emoji-search-input"
        />
      </div>
      {!search && (
        <div className="emoji-category-tabs">
          {EMOJI_CATEGORIES.map((cat, i) => (
            <button
              key={cat.name}
              className={`emoji-cat-btn${activeCategory === i ? ' active' : ''}`}
              onClick={() => setActiveCategory(i)}
              title={cat.name}
            >
              {cat.icon}
            </button>
          ))}
        </div>
      )}
      <div className="emoji-grid">
        {displayEmojis.map((emoji, i) => (
          <button key={i} className="emoji-grid-btn" onClick={() => onSelect(emoji)}>
            {emoji}
          </button>
        ))}
      </div>
    </div>
  );
};
```

- [ ] **Step 2: Verify build**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build 2>&1 | tail -10
```
Expected: no errors.

- [ ] **Step 3: Manual test**

Open the app, click the emoji button in chat, type "smile" in the search box. Expected: emoji results appear (😊, 😄, etc.).

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/chat/EmojiPicker.tsx
git commit -m "fix: emoji picker search using emoji-mart SearchIndex"
```

---

### Task 7: Frontend — Audio bitrate 24 kbps

**Files:**
- Modify: `frontend/src/hooks/useVoiceConnection.ts:242`

**Context:** Line 242 is `await room.localParticipant.publishTrack(micTrack);`. Adding `{ audioBitrate: 24_000 }` as the second argument tells LiveKit to encode Opus at 24 kbps instead of the default 32 kbps.

- [ ] **Step 1: Update publishTrack call**

In `frontend/src/hooks/useVoiceConnection.ts`, change line 242 from:
```typescript
await room.localParticipant.publishTrack(micTrack);
```
to:
```typescript
await room.localParticipant.publishTrack(micTrack, { audioBitrate: 24_000 });
```

- [ ] **Step 2: Verify build**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build 2>&1 | tail -10
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/hooks/useVoiceConnection.ts
git commit -m "perf: set voice audio bitrate to 24kbps (Opus, saves ~25% vs default)"
```

---

## Final Build Verification

After all tasks are complete:

- [ ] **Full Go build**

```bash
cd /home/dylan/Developer/parley && go build ./...
```
Expected: no errors.

- [ ] **Full frontend build**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build 2>&1 | tail -20
```
Expected: no TypeScript errors, build succeeds.
