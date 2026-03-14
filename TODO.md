# Parley TODO

This is a living task list for Parley - a Discord clone.

## High Priority

### Completed
- [x] Redis pub/sub for cross-node WebSocket broadcasting (3 droplets: 138.197.83.70, 138.197.97.235, 165.227.121.15)
- [x] User settings context menu (click username at bottom-left shows menu)
- [x] User settings modal (change username/password)
- [x] Server member join events broadcast via WebSocket

### Pending

- [ ] Right-click context menu on usernames in chat (channel messages)
- [ ] Right-click context menu on usernames in sidebar (user sidebar)
- [ ] DM from search doesn't show for the sender
- [ ] User joining server doesn't refresh server sidebar for others immediately
- [ ] Message editing in voice channels
- [ ] Delete/edit messages in VC

---

## Medium Priority

### Features
- [ ] Typing indicators
- [ ] Unread message badges on servers and DM channels
- [ ] Real-time online status indicators
- [ ] Channel topics and descriptions
- [ ] Message reactions
- [ ] Emoji picker
- [ ] Image/file upload in messages
- [ ] Server pictures and user profile pictures (DigitalOcean Spaces)
- [ ] User banners (PNG/JPG/animated GIF support with ideal size recommendation)

### Infrastructure
- [ ] CI/CD deploy script (auto-deploy on push to main)
- [ ] WebSocket reconnection with exponential backoff

---

## Lower Priority

- [ ] Voice channels/voice chat (high priority but complex - need voice server architecture decision)
- [ ] Server discovery / public servers list
- [ ] Message search
- [ ] Notification system (browser push + in-app)
- [ ] User profile page with custom display name

---

## Future / Large Projects

- [ ] Admin panel for service administration
  - Full observability and administrative capabilities
  - Ban users (dissolve accounts with funny error message)
  - View logs/metrics

- [ ] Server-wide permissions/privileges system
  - Tab in server settings to control these
  - Multiple roles per user
  - Custom role interface (not browser dropdown)

- [ ] Passkey authentication
  - Configurable in user settings
  - Logic to determine passkey vs password before prompting

- [ ] 2FA (Google Authenticator first)
  - Large undertaking

- [ ] Message search in messages

---

## Already Implemented but Need Verification/Adjustment

- [ ] User-profile-username CSS (top margin issue - user fixed, need verification)
- [ ] Server settings modal (delete button working?)
- [ ] Channel URLs (URL-based navigation - refresh preserves position)
