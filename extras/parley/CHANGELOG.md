# Changelog

## 1.1.0 — 2026-04-27

Major endpoint coverage expansion + packaging.

### Added
- Friend / block system: send/accept/reject friend requests, block/unblock, list
- DM groups: create, add/remove members, leave
- Notifications: list, mark read, mark all read
- Member roles: assign/remove
- Voice: token retrieval, join/leave/kick/mute, list participants, ring
- Soundboard: list/upload/delete/play
- Themes: list/install/get
- Bin (anonymous posts): create/list/delete
- Uploads: typed wrappers for avatar/banner/server-icon
- Passkey: register/list/delete (login flow noted as pre-auth, requires separate code path)
- GDPR: account export, account delete
- Channel permission overwrites: get/set/delete
- Message extras: search, forward, reactions list, reaction remove

### Changed
- Packaged as pip-installable distribution (`pip install parley`)

## 1.0.0 — initial release

Bot, Selfbot, CommandBot, Cogs, Commands, Slash, Gateway, base models.
