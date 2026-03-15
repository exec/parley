# Channels

## Channel Types

| Value | Type |
|---|---|
| `0` | Text channel |
| `1` | Voice channel |

---

## Get Channel

<span class="method get">GET</span><span class="route">/api/channels/{channelId}</span>

**Response** `200 OK`

```json
{
  "id": "123456789",
  "server_id": "987654321",
  "name": "general",
  "type": 0,
  "position": 0,
  "created_at": "2026-03-14T12:00:00Z"
}
```

---

## Get Server Channels

<span class="method get">GET</span><span class="route">/api/servers/{serverId}/channels</span>

Returns all channels in a server. You must be a member.

**Response** `200 OK` — array of channel objects.

---

## Create Channel

<span class="method post">POST</span><span class="route">/api/servers/{serverId}/channels</span>

Requires admin/owner permissions in the server.

**Request body**

```json
{
  "name": "announcements",
  "type": 0
}
```

**Response** `201 Created` — channel object.

---

## Update Channel

<span class="method put">PUT</span><span class="route">/api/channels/{channelId}</span>

Requires admin/owner permissions.

**Request body**

```json
{ "name": "new-name" }
```

**Response** `200 OK` — updated channel object.

---

## Delete Channel

<span class="method delete">DELETE</span><span class="route">/api/channels/{channelId}</span>

Requires admin/owner permissions.

**Response** `204 No Content`

---

## Channel Object

```ts
interface Channel {
  id: string
  server_id: string
  name: string
  type: 0 | 1         // 0 = text, 1 = voice
  position: number
  created_at: string
}
```
