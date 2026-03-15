# Authentication

All API requests (except auth endpoints) require an `Authorization` header:

```http
Authorization: Bearer <token>
```

There are two token types:

## API Keys (`plk_…`)

Generated from **User Settings → Developer**. API keys always start with `plk_` followed by 40 hex characters (44 characters total).

```
plk_3f9a2b1c8d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a
```

There are two kinds of API key:

| | **Bot Key** | **User Key (Selfbot)** |
|---|---|---|
| Authenticates as | A new bot user you create | You (the key owner) |
| Chat badge | <span style="color:#c864ff">**BOT**</span> purple badge | 🤖 selfbot icon |
| `via_api` flag | `true` | `true` |
| `author_is_bot` flag | `true` | `false` |

### Using an API key

Pass the key exactly like a JWT in the `Authorization` header:

```http
GET /api/channels/123/messages
Authorization: Bearer plk_3f9a2b1c8d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a
```

For WebSocket connections, pass it as a query parameter (browsers can't set headers on `WebSocket`):

```
wss://parley.x86-64.com/ws?token=plk_3f9a2b1c8d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a
```

## Errors

| Code | Meaning |
|------|---------|
| `401` | Missing, invalid, or expired token |
| `403` | Authenticated but not permitted (e.g. not a member of this server) |

```json
{ "message": "Invalid or expired token" }
```
