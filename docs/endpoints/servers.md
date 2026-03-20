# Servers

## List Servers

<span class="method get">GET</span><span class="route">/api/servers</span>

Returns all servers the authenticated user (or bot) is a member of.

**Response** `200 OK`

```json
[
  {
    "id": "123456789",
    "name": "My Server",
    "icon_url": "https://cdn.parley.x86-64.com/uploads/icon.png",
    "owner_id": "111",
    "vanity_url": "my-server",
    "created_at": "2026-01-01T00:00:00Z",
    "updated_at": "2026-01-01T00:00:00Z"
  }
]
```

---

## Get Server

<span class="method get">GET</span><span class="route">/api/servers/{serverId}</span>

**Response** `200 OK` — server object.

---

## Create Server

<span class="method post">POST</span><span class="route">/api/servers</span>

**Request body**

```json
{
  "name": "My Server",
  "icon_url": "https://example.com/icon.png"
}
```

**Response** `201 Created` — server object.

---

## Update Server

<span class="method put">PUT</span><span class="route">/api/servers/{serverId}</span>

Requires owner permissions.

**Request body**

```json
{
  "name": "New Name",
  "icon_url": "https://example.com/new-icon.png"
}
```

**Response** `200 OK` — updated server object.

---

## Delete Server

<span class="method delete">DELETE</span><span class="route">/api/servers/{serverId}</span>

Requires owner. Permanently deletes the server and all its channels and messages.

**Response** `204 No Content`

---

## Members

### Get Members

<span class="method get">GET</span><span class="route">/api/servers/{serverId}/members</span>

**Response** `200 OK` — array of member objects.

### Add Bot to Server

<span class="method post">POST</span><span class="route">/api/servers/{serverId}/members</span>

Adds a **bot user** to a server. Requires the caller to be the server owner or have the **Administrator** permission. This endpoint exists specifically for bot onboarding — regular users join via invite links, not this API.

```json
{ "user_id": "<bot-user-id>" }
```

The `bot_user_id` is returned when you create a Bot API Key (see [API Keys](/endpoints/developer)).

**Response** `201 Created`

```json
{ "server_id": "987", "user_id": "222", "nickname": "" }
```

### Remove Member

<span class="method delete">DELETE</span><span class="route">/api/servers/{serverId}/members/{userId}</span>

**Response** `204 No Content`

### Leave Server

<span class="method delete">DELETE</span><span class="route">/api/servers/{serverId}/leave</span>

**Response** `204 No Content`

### Kick Member

<span class="method post">POST</span><span class="route">/api/servers/{serverId}/members/{userId}/kick</span>

Requires admin/owner permissions. **Response** `204 No Content`

### Ban Member

<span class="method post">POST</span><span class="route">/api/servers/{serverId}/members/{userId}/ban</span>

Requires admin/owner permissions. **Response** `204 No Content`

---

## Roles

### Get Roles

<span class="method get">GET</span><span class="route">/api/servers/{serverId}/roles</span>

### Create Role

<span class="method post">POST</span><span class="route">/api/servers/{serverId}/roles</span>

```json
{
  "name": "Moderator",
  "color": "#ff0000",
  "permissions": 0
}
```

### Update Role

<span class="method patch">PATCH</span><span class="route">/api/servers/{serverId}/roles/{roleId}</span>

### Delete Role

<span class="method delete">DELETE</span><span class="route">/api/servers/{serverId}/roles/{roleId}</span>

### Assign Role to Member

<span class="method post">POST</span><span class="route">/api/servers/{serverId}/members/{userId}/roles</span>

```json
{ "role_id": "333" }
```

### Remove Role from Member

<span class="method delete">DELETE</span><span class="route">/api/servers/{serverId}/members/{userId}/roles/{roleId}</span>

---

## Invites

### Create Invite

<span class="method post">POST</span><span class="route">/api/servers/{serverId}/invites</span>

**Response** `201 Created`

```json
{ "code": "abc123", "server_id": "123", "expires_at": null }
```

### Get Invite

<span class="method get">GET</span><span class="route">/api/invites/{code}</span>

Returns server info for the invite (no auth required).

### Set Vanity URL

<span class="method put">PUT</span><span class="route">/api/servers/{serverId}/vanity</span>

Requires owner. Sets a custom invite code slug.

```json
{ "code": "my-server" }
```
