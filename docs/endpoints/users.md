# Users

## Get Current User

<span class="method get">GET</span><span class="route">/api/auth/me</span>

Returns the full profile of the authenticated user. For a Bot API Key, returns the bot user's profile.

**Response** `200 OK`

```json
{
  "id": "123456789",
  "username": "dylan",
  "display_name": "Dylan",
  "email": "dylan@example.com",
  "avatar_url": "https://cdn.parley.x86-64.com/uploads/avatar.png",
  "banner_url": "",
  "bio": "Hello world",
  "badges": 0,
  "email_verified": true,
  "phone_number": "",
  "phone_verified": false,
  "status_type": "online",
  "status_text": ""
}
```

::: info Bot API Key
When authenticating with a Bot Key, `email` will be an internal placeholder (`bot_<hex>@internal.parley`) and `email_verified` will be `false`. Only your real account has a real email.
:::

---

## Get User Profile

<span class="method get">GET</span><span class="route">/api/users/{userId}</span>

Returns a user's public profile.

**Response** `200 OK`

```json
{
  "id": "123456789",
  "username": "dylan",
  "display_name": "Dylan",
  "avatar_url": "https://cdn.parley.x86-64.com/uploads/avatar.png",
  "banner_url": "",
  "bio": "Hello world",
  "badges": 0,
  "created_at": "2026-01-01T00:00:00Z"
}
```

---

## Search Users

<span class="method get">GET</span><span class="route">/api/users/search?q={query}</span>

Search users by username prefix.

**Query parameters**

| Parameter | Required | Description |
|---|---|---|
| `q` | ✅ | Username prefix to search (min 1 char) |

**Response** `200 OK` — array of public user objects.

---

## Update Profile

<span class="method put">PUT</span><span class="route">/api/auth/profile</span>

Updates the authenticated user's profile. For a Bot API Key, updates the bot user.

```json
{
  "username": "new-username",
  "current_password": "existing-password",
  "new_password": "new-password",
  "avatar_url": "https://cdn.parley.x86-64.com/uploads/new-avatar.png",
  "banner_url": "https://cdn.parley.x86-64.com/uploads/banner.png",
  "bio": "Updated bio"
}
```

All fields are optional — only provided fields are updated. `current_password` is required when changing `new_password`.

**Response** `200 OK` — updated user object.
