# Rate Limits & Request Limits

## Rate Limits

| Endpoint group | Limit |
|---|---|
| `POST /api/auth/register` | 10 requests / IP / minute |
| `POST /api/auth/login` | 10 requests / IP / minute |
| `GET /api/auth/verify-email` | 10 requests / IP / minute |
| All other endpoints | No platform-wide limit currently |

Rate limited responses return `429 Too Many Requests`.

## Request Body Limits

| Endpoint | Max body size |
|---|---|
| `POST /api/upload` | 25 MB |
| All other endpoints | 64 KB |

Exceeding the limit returns `413 Request Entity Too Large`.

## Message Content

- Either `content` or `attachment_url` must be present (or both)
- No enforced character limit beyond the 64 KB body cap

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
