# Parley TODO

## Bugs
- [ ] Large emoji (1–5 emoji-only messages should render at 2x size) — detection logic not working, needs debug

## Planned

### Permissions System (In Progress)
- [ ] Full permission overhaul — 42-bit permission system, role hierarchy, channel overwrites, category sync
- [ ] Discord bridge — bridge Parley channels to Discord channels bidirectionally (see DISCORD_BRIDGE.md)

### Future Features (Permission Bits Defined)
- [ ] Audit Log — server activity log with filters (ViewAuditLog permission)
- [ ] Member Timeout — temporarily restrict a member's permissions (ModerateMember permission)
- [ ] Webhooks — incoming/outgoing webhooks per channel (ManageWebhooks permission)
- [ ] Custom Emoji & Stickers — server-level custom expressions (ManageExpressions permission)
- [ ] Server Events — scheduled events in voice/text channels (ManageEvents permission)
- [ ] External Emoji — use emoji from other servers in messages (UseExternalEmoji permission)
- [ ] Threads — threaded conversations in text channels (ManageThreads, CreatePublicThreads, SendMessagesInThreads permissions)
- [ ] Priority Speaker — lower volume for others when speaking in voice (PrioritySpeaker permission)
- [ ] Screen Sharing / Go Live — stream screen in voice channels (Stream permission)
- [ ] Soundboard — shared sound effects in voice channels (UseSoundboard permission)
- [ ] Voice Messages — audio messages in text and voice channels (SendVoiceMessages permission)
- [ ] Push-to-Talk Enforcement — require PTT vs voice activity detection (UseVAD permission)
- [ ] Nickname Management — change own and others' nicknames (ChangeNickname, ManageNicknames permissions)
