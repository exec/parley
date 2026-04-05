# Rate Limits & Request Limits

## Rate Limits

| Endpoint group | Limit | Key |
|---|---|---|
| `POST /api/auth/register` | 10 requests / minute | IP |
| `POST /api/auth/login` | 10 requests / minute | IP |
| `GET /api/auth/verify-email` | 10 requests / minute | IP |
| Message writes (`POST /channels/{id}/messages`) | 10 requests / 2 seconds | authenticated user |
| Message reads (`GET /channels/{id}/messages`) | 120 requests / minute | IP |
| Message search (`GET /servers/{id}/messages/search`) | 20 requests / minute | authenticated user |
| Discovery (`GET /discover`, `GET /server-categories`) | 30 requests / minute | IP |
| Invite lookups (`GET|POST /invites/{code}`) | 30 requests / minute | IP |
| File uploads (`POST /api/upload`) | 30 uploads / hour | authenticated user |

Rate limited responses return `429 Too Many Requests`.

## Request Body Limits

| Endpoint | Max body size |
|---|---|
| `POST /api/upload` | 50 MB |
| `POST /servers/{id}/soundboard` | 1 MB |
| All other endpoints | 64 KB |

Exceeding the limit returns `413 Request Entity Too Large`.

## Message Content

- Either `content` or `attachment_url` must be present (or both)
- Maximum content length: **4,000 characters** (regular users), **8,000 characters** (bots)

## Pagination

Endpoints that return lists (messages, members, etc.) accept `limit` and `offset` query parameters.

| Parameter | Default | Max |
|---|---|---|
| `limit` | 50 | 100 |
| `offset` | 0 | — |

## Response Format

All responses are JSON with `Content-Type: application/json`.

Errors follow this shape:

```json
{
  "message": "human-readable error description"
}
```

IDs are returned as **strings** in all responses (e.g. `"id": "123456789"`) for JavaScript compatibility, even though they are 64-bit integers internally.
