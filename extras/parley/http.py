"""
parley.http
===========
Low-level async HTTP client for the Parley REST API.

This module is intentionally model-free: it deals only in raw dicts and
primitive types.  The :class:`~parley.state.ConnectionState` layer is
responsible for converting those dicts into model objects.

All methods raise subclasses of :class:`~parley.errors.HTTPError` on
non-2xx responses.
"""

from __future__ import annotations

import logging
from typing import Any, Optional

import httpx

from .errors import (
    AuthError,
    ForbiddenError,
    HTTPError,
    NotFoundError,
    RateLimitError,
    ServerError,
)

__all__ = ["HTTPClient"]

log = logging.getLogger("parley.http")

_EMPTY_DICT: dict = {}


def _raise_for_status(resp: httpx.Response) -> None:
    """Inspect *resp* and raise an appropriate :class:`HTTPError` if needed."""
    if resp.status_code < 300:
        return
    try:
        body = resp.json()
        msg: str = body.get("error") or body.get("message") or resp.text
    except Exception:
        msg = resp.text or f"HTTP {resp.status_code}"

    code = resp.status_code
    if code == 401:
        raise AuthError(msg, code)
    if code == 403:
        raise ForbiddenError(msg, code)
    if code == 404:
        raise NotFoundError(msg, code)
    if code == 429:
        raise RateLimitError(msg, code)
    if code >= 500:
        raise ServerError(msg, code)
    raise HTTPError(msg, code)


class HTTPClient:
    """
    Async HTTP client wrapping all Parley REST endpoints.

    Parameters
    ----------
    base_url:
        Root URL of the Parley instance, e.g. ``https://parley.x86-64.com``.
    token:
        Bearer token (JWT *or* ``plk_…`` API key).
    timeout:
        Per-request timeout in seconds.
    """

    def __init__(self, base_url: str, token: str, *, timeout: float = 30.0) -> None:
        self.base_url = base_url.rstrip("/")
        self._token = token
        self._timeout = timeout
        self._client: Optional[httpx.AsyncClient] = None

    # ------------------------------------------------------------------
    # Lifecycle
    # ------------------------------------------------------------------

    async def _ensure_client(self) -> httpx.AsyncClient:
        if self._client is None or self._client.is_closed:
            self._client = httpx.AsyncClient(
                base_url=self.base_url,
                headers={
                    "Authorization": f"Bearer {self._token}",
                    "Content-Type": "application/json",
                    "Accept": "application/json",
                },
                timeout=self._timeout,
            )
        return self._client

    async def close(self) -> None:
        """Close the underlying HTTP connection pool."""
        if self._client and not self._client.is_closed:
            await self._client.aclose()
        self._client = None

    # ------------------------------------------------------------------
    # Low-level request helpers
    # ------------------------------------------------------------------

    async def request(
        self,
        method: str,
        path: str,
        *,
        json: Any = None,
        params: Optional[dict] = None,
        data: Any = None,
        files: Any = None,
        extra_headers: Optional[dict] = None,
    ) -> Any:
        """Issue an HTTP request and return the parsed JSON body (or ``None``).

        Parameters
        ----------
        method:
            HTTP verb (``GET``, ``POST``, …).
        path:
            Path relative to *base_url*, e.g. ``/api/auth/me``.
        json:
            JSON-serialisable body to send.
        params:
            Query-string parameters; ``None`` values are stripped.
        data:
            Raw form data (used for multipart uploads).
        files:
            File mapping for multipart uploads.
        extra_headers:
            Additional headers to merge for this request only.
        """
        client = await self._ensure_client()
        clean_params = (
            {k: v for k, v in params.items() if v is not None} if params else None
        )
        headers = extra_headers or {}
        log.debug("%s %s  params=%s", method, path, clean_params)
        resp = await client.request(
            method,
            path,
            json=json,
            params=clean_params,
            data=data,
            files=files,
            headers=headers,
        )
        _raise_for_status(resp)
        if resp.status_code == 204 or not resp.content:
            return None
        return resp.json()

    # Convenience shorthands
    async def get(self, path: str, **params: Any) -> Any:
        return await self.request("GET", path, params=params)

    async def post(self, path: str, body: Optional[dict] = None) -> Any:
        return await self.request("POST", path, json=body or _EMPTY_DICT)

    async def put(self, path: str, body: Optional[dict] = None) -> Any:
        return await self.request("PUT", path, json=body or _EMPTY_DICT)

    async def patch(self, path: str, body: Optional[dict] = None) -> Any:
        return await self.request("PATCH", path, json=body or _EMPTY_DICT)

    async def delete(self, path: str) -> None:
        await self.request("DELETE", path)

    # ------------------------------------------------------------------
    # Auth
    # ------------------------------------------------------------------

    async def get_me(self) -> dict:
        """``GET /api/users/me`` — current user profile."""
        return await self.get("/api/users/me")

    async def login(self, email: str, password: str) -> dict:
        """``POST /api/auth/login`` — returns ``{token, user}``."""
        return await self.post("/api/auth/login", {"email": email, "password": password})

    async def get_ws_ticket(self) -> str:
        """``POST /api/auth/ws-ticket`` — returns a short-lived WS ticket string."""
        data = await self.post("/api/auth/ws-ticket")
        return data["ticket"]

    # ------------------------------------------------------------------
    # Users
    # ------------------------------------------------------------------

    async def get_user(self, user_id: int) -> dict:
        """``GET /api/users/{id}``"""
        return await self.get(f"/api/users/{user_id}")

    async def search_users(self, query: str) -> list[dict]:
        """``GET /api/users/search?q=...``"""
        return await self.get("/api/users/search", q=query)

    async def edit_me(self, **fields) -> dict:
        """``PATCH /api/users/me`` — update username, display_name, or avatar_url."""
        allowed = {"username", "display_name", "avatar_url"}
        body = {k: v for k, v in fields.items() if k in allowed}
        return await self.request("PATCH", "/api/users/me", json=body)

    async def set_status(self, status_type: str, text: str = "") -> None:
        """``PATCH /api/users/@me/status`` — set status_type and status_text."""
        await self.request("PATCH", "/api/users/@me/status", json={
            "status_type": status_type,
            "status_text": text,
        })

    async def send_typing(self, channel_id: int, duration: int = 5) -> None:
        """``POST /api/channels/{id}/typing`` — notify typing for up to *duration* seconds."""
        await self.request("POST", f"/api/channels/{channel_id}/typing", json={
            "duration": max(1, min(60, duration)),
        })

    async def update_bot_invite(self, bot_id: int, *, permissions: Optional[int] = None,
                                show_author: Optional[bool] = None) -> dict:
        """``PATCH /api/developer/bots/{id}/invite`` — update bot invite settings."""
        body: dict = {}
        if permissions is not None:
            body["permissions"] = permissions
        if show_author is not None:
            body["show_author"] = show_author
        return await self.request("PATCH", f"/api/developer/bots/{bot_id}/invite", json=body)

    # ------------------------------------------------------------------
    # Servers
    # ------------------------------------------------------------------

    async def get_servers(self) -> list[dict]:
        """``GET /api/servers``"""
        return await self.get("/api/servers")

    async def get_server(self, server_id: int) -> dict:
        return await self.get(f"/api/servers/{server_id}")

    async def create_server(self, name: str) -> dict:
        """``POST /api/servers``"""
        return await self.post("/api/servers", {"name": name})

    async def edit_server(
        self,
        server_id: int,
        *,
        name: Optional[str] = None,
        icon_url: Optional[str] = None,
    ) -> dict:
        """``PUT /api/servers/{id}``"""
        body: dict = {}
        if name is not None:
            body["name"] = name
        if icon_url is not None:
            body["icon_url"] = icon_url
        return await self.put(f"/api/servers/{server_id}", body)

    async def delete_server(self, server_id: int) -> None:
        """``DELETE /api/servers/{id}``"""
        await self.delete(f"/api/servers/{server_id}")

    async def leave_server(self, server_id: int) -> None:
        """``DELETE /api/servers/{id}/leave``"""
        await self.delete(f"/api/servers/{server_id}/leave")

    # ------------------------------------------------------------------
    # Members
    # ------------------------------------------------------------------

    async def get_server_members(self, server_id: int) -> list[dict]:
        return await self.get(f"/api/servers/{server_id}/members")

    async def add_server_member(self, server_id: int, user_id: int) -> dict:
        return await self.post(
            f"/api/servers/{server_id}/members", {"user_id": user_id}
        )

    async def remove_server_member(self, server_id: int, user_id: int) -> None:
        await self.delete(f"/api/servers/{server_id}/members/{user_id}")

    async def kick_member(self, server_id: int, user_id: int) -> None:
        await self.post(f"/api/servers/{server_id}/members/{user_id}/kick")

    async def ban_member(self, server_id: int, user_id: int) -> None:
        await self.post(f"/api/servers/{server_id}/members/{user_id}/ban")

    # ------------------------------------------------------------------
    # Roles
    # ------------------------------------------------------------------

    async def get_roles(self, server_id: int) -> list[dict]:
        return await self.get(f"/api/servers/{server_id}/roles")

    async def create_role(
        self,
        server_id: int,
        *,
        name: str,
        color: str = "",
        permissions: int = 0,
        hoist: bool = False,
    ) -> dict:
        return await self.post(
            f"/api/servers/{server_id}/roles",
            {"name": name, "color": color, "permissions": permissions, "hoist": hoist},
        )

    async def edit_role(self, server_id: int, role_id: int, **fields: Any) -> dict:
        return await self.patch(
            f"/api/servers/{server_id}/roles/{role_id}", fields
        )

    async def delete_role(self, server_id: int, role_id: int) -> None:
        await self.delete(f"/api/servers/{server_id}/roles/{role_id}")

    async def add_member_role(
        self, server_id: int, user_id: int, role_id: int
    ) -> None:
        await self.post(
            f"/api/servers/{server_id}/members/{user_id}/roles",
            {"role_id": role_id},
        )

    async def remove_member_role(
        self, server_id: int, user_id: int, role_id: int
    ) -> None:
        await self.delete(
            f"/api/servers/{server_id}/members/{user_id}/roles/{role_id}"
        )

    # ------------------------------------------------------------------
    # Invites
    # ------------------------------------------------------------------

    async def get_server_invites(self, server_id: int) -> list[dict]:
        return await self.get(f"/api/servers/{server_id}/invites")

    async def create_invite(self, server_id: int) -> dict:
        return await self.post(f"/api/servers/{server_id}/invites")

    async def get_invite(self, code: str) -> dict:
        """``GET /api/invites/{code}`` — no authentication required."""
        return await self.get(f"/api/invites/{code}")

    # ------------------------------------------------------------------
    # Channels
    # ------------------------------------------------------------------

    async def get_server_channels(self, server_id: int) -> list[dict]:
        return await self.get(f"/api/servers/{server_id}/channels")

    async def create_channel(
        self,
        server_id: int,
        *,
        name: str,
        channel_type: int = 0,
        topic: Optional[str] = None,
        parent_id: Optional[int] = None,
    ) -> dict:
        body: dict = {"name": name, "channel_type": channel_type}
        if topic is not None:
            body["topic"] = topic
        if parent_id is not None:
            body["parent_id"] = parent_id
        return await self.post(f"/api/servers/{server_id}/channels", body)

    async def get_channel(self, channel_id: int) -> dict:
        return await self.get(f"/api/channels/{channel_id}")

    async def edit_channel(
        self,
        channel_id: int,
        *,
        name: Optional[str] = None,
        topic: Optional[str] = None,
    ) -> dict:
        body: dict = {}
        if name is not None:
            body["name"] = name
        if topic is not None:
            body["topic"] = topic
        return await self.put(f"/api/channels/{channel_id}", body)

    async def delete_channel(self, channel_id: int) -> None:
        await self.delete(f"/api/channels/{channel_id}")

    # ------------------------------------------------------------------
    # Messages
    # ------------------------------------------------------------------

    async def get_messages(
        self,
        channel_id: int,
        *,
        limit: int = 50,
        before: Optional[int] = None,
    ) -> list[dict]:
        return await self.get(
            f"/api/channels/{channel_id}/messages",
            limit=limit,
            before=before,
        )

    async def create_message(
        self,
        channel_id: int,
        content: str,
        *,
        parent_id: Optional[int] = None,
    ) -> dict:
        body: dict = {"content": content}
        if parent_id is not None:
            body["parent_id"] = parent_id
        return await self.post(f"/api/channels/{channel_id}/messages", body)

    async def edit_message(self, message_id: int, content: str) -> dict:
        return await self.put(f"/api/messages/{message_id}", {"content": content})

    async def delete_message(self, message_id: int) -> None:
        await self.delete(f"/api/messages/{message_id}")

    async def add_reaction(self, message_id: int, emoji: str) -> None:
        """``POST /api/messages/{id}/reactions`` — toggles a reaction."""
        await self.post(f"/api/messages/{message_id}/reactions", {"emoji": emoji})

    # ------------------------------------------------------------------
    # DMs
    # ------------------------------------------------------------------

    async def get_dms(self) -> list[dict]:
        return await self.get("/api/dms")

    async def open_dm(self, user_id: int) -> dict:
        return await self.post("/api/dms", {"user_id": user_id})

    async def get_dm_messages(
        self,
        dm_channel_id: int,
        *,
        limit: int = 50,
        before: Optional[int] = None,
    ) -> list[dict]:
        return await self.get(
            f"/api/dms/{dm_channel_id}/messages",
            limit=limit,
            before=before,
        )

    async def create_dm_message(self, dm_channel_id: int, content: str) -> dict:
        return await self.post(
            f"/api/dms/{dm_channel_id}/messages", {"content": content}
        )

    # ------------------------------------------------------------------
    # File upload
    # ------------------------------------------------------------------

    async def upload_file(self, file_bytes: bytes, filename: str) -> str:
        """``POST /api/upload`` — upload a file and return its URL.

        Parameters
        ----------
        file_bytes:
            Raw bytes of the file to upload.
        filename:
            Filename to use in the multipart form.

        Returns
        -------
        str
            The public URL of the uploaded file.
        """
        client = await self._ensure_client()
        # Upload requires multipart — send without JSON Content-Type header.
        resp = await client.post(
            "/api/upload",
            files={"file": (filename, file_bytes)},
            headers={"Authorization": f"Bearer {self._token}"},
        )
        _raise_for_status(resp)
        return resp.json()["url"]

    # ------------------------------------------------------------------
    # Developer keys
    # ------------------------------------------------------------------

    async def get_api_keys(self) -> list[dict]:
        """``GET /api/developer/keys``"""
        return await self.get("/api/developer/keys")

    async def create_api_key(self, name: str) -> dict:
        """``POST /api/developer/keys``"""
        return await self.post("/api/developer/keys", {"name": name})

    async def delete_api_key(self, key_id: int) -> None:
        """``DELETE /api/developer/keys/{id}``"""
        await self.delete(f"/api/developer/keys/{key_id}")

    # ==================================================================
    # ===== Friend / DM-group / Notification / Member-role =====
    # ==================================================================

    # ------------------------------------------------------------------
    # Friends
    # ------------------------------------------------------------------

    async def get_friends(self) -> list[dict]:
        """``GET /api/friends`` — accepted friendships."""
        return await self.get("/api/friends")

    async def get_friend_requests(self) -> dict:
        """``GET /api/friend-requests`` — ``{incoming: [...], outgoing: [...]}``."""
        return await self.get("/api/friend-requests")

    async def send_friend_request(self, username: str) -> dict:
        """``POST /api/friend-requests`` — send a request by *username*."""
        return await self.post("/api/friend-requests", {"username": username})

    async def accept_friend_request(self, request_id: int) -> dict:
        """``POST /api/friend-requests/{id}/accept`` — returns the new friend."""
        return await self.post(f"/api/friend-requests/{request_id}/accept")

    async def decline_friend_request(self, request_id: int) -> None:
        """``DELETE /api/friend-requests/{id}`` — decline (incoming) or cancel (outgoing)."""
        await self.delete(f"/api/friend-requests/{request_id}")

    async def cancel_friend_request(self, request_id: int) -> None:
        """Alias for :meth:`decline_friend_request` — cancel an outgoing request."""
        await self.delete(f"/api/friend-requests/{request_id}")

    async def remove_friend(self, user_id: int) -> None:
        """``DELETE /api/friends/{userId}`` — unfriend a user."""
        await self.delete(f"/api/friends/{user_id}")

    # ------------------------------------------------------------------
    # Blocks
    # ------------------------------------------------------------------

    async def get_blocks(self) -> list[dict]:
        """``GET /api/blocks`` — list blocked users."""
        return await self.get("/api/blocks")

    async def block_user(self, user_id: int) -> None:
        """``POST /api/users/{id}/block``"""
        await self.post(f"/api/users/{user_id}/block")

    async def unblock_user(self, user_id: int) -> None:
        """``DELETE /api/users/{id}/block``"""
        await self.delete(f"/api/users/{user_id}/block")

    # ------------------------------------------------------------------
    # DM groups
    # ------------------------------------------------------------------

    async def create_dm_group(
        self,
        user_ids: list[int],
        *,
        name: Optional[str] = None,
    ) -> dict:
        """``POST /api/dms`` — open a group DM with multiple users.

        For 1:1 DMs, prefer :meth:`open_dm`.
        """
        body: dict = {"user_ids": [str(uid) for uid in user_ids]}
        if name is not None:
            body["name"] = name
        return await self.post("/api/dms", body)

    async def get_dm_members(self, dm_channel_id: int) -> list[dict]:
        """``GET /api/dms/{id}/members``"""
        return await self.get(f"/api/dms/{dm_channel_id}/members")

    async def add_dm_members(
        self, dm_channel_id: int, user_ids: list[int]
    ) -> None:
        """``POST /api/dms/{id}/members`` — add users to a group DM."""
        await self.post(
            f"/api/dms/{dm_channel_id}/members",
            {"user_ids": [str(uid) for uid in user_ids]},
        )

    async def add_dm_member(self, dm_channel_id: int, user_id: int) -> None:
        """Convenience wrapper around :meth:`add_dm_members` for a single user."""
        await self.add_dm_members(dm_channel_id, [user_id])

    async def remove_dm_member(self, dm_channel_id: int, user_id: int) -> None:
        """``DELETE /api/dms/{id}/members/{userID}`` — owner-only kick."""
        await self.delete(f"/api/dms/{dm_channel_id}/members/{user_id}")

    async def leave_dm(
        self,
        dm_channel_id: int,
        *,
        transfer_to: Optional[int] = None,
    ) -> None:
        """``POST /api/dms/{id}/leave``.

        Parameters
        ----------
        dm_channel_id:
            Group DM channel ID.
        transfer_to:
            If the actor is the owner, transfer ownership to this user before leaving.
        """
        body: dict = {}
        if transfer_to is not None:
            body["transfer_to"] = str(transfer_to)
        await self.request("POST", f"/api/dms/{dm_channel_id}/leave", json=body)

    async def update_dm_group(
        self,
        dm_channel_id: int,
        *,
        name: Optional[str] = None,
        avatar_url: Optional[str] = None,
        clear_avatar: bool = False,
    ) -> None:
        """``PATCH /api/dms/{id}`` — rename and/or update avatar of a group DM."""
        body: dict = {}
        if name is not None:
            body["name"] = name
        if avatar_url is not None:
            body["avatar_url"] = avatar_url
        if clear_avatar:
            body["clear_avatar"] = True
        await self.patch(f"/api/dms/{dm_channel_id}", body)

    async def transfer_dm_ownership(
        self, dm_channel_id: int, new_owner_id: int
    ) -> None:
        """``POST /api/dms/{id}/transfer-ownership``"""
        await self.post(
            f"/api/dms/{dm_channel_id}/transfer-ownership",
            {"new_owner_id": str(new_owner_id)},
        )

    # ------------------------------------------------------------------
    # Notifications
    # ------------------------------------------------------------------

    async def get_notifications(self, *, limit: int = 50) -> list[dict]:
        """``GET /api/notifications?limit=N``"""
        return await self.get("/api/notifications", limit=limit)

    async def mark_notification_read(self, notification_id: int) -> None:
        """``PATCH /api/notifications/{id}/read``"""
        await self.patch(f"/api/notifications/{notification_id}/read")

    async def mark_all_notifications_read(self) -> None:
        """``PATCH /api/notifications/read-all``"""
        await self.patch("/api/notifications/read-all")

    # ------------------------------------------------------------------
    # Member roles
    # ------------------------------------------------------------------

    async def get_member_roles(self, server_id: int, user_id: int) -> list[dict]:
        """``GET /api/servers/{id}/members/{userID}/roles``"""
        return await self.get(
            f"/api/servers/{server_id}/members/{user_id}/roles"
        )

    async def assign_role_to_member(
        self, server_id: int, user_id: int, role_id: int
    ) -> None:
        """``POST /api/servers/{id}/members/{userID}/roles`` — assign a role."""
        await self.post(
            f"/api/servers/{server_id}/members/{user_id}/roles",
            {"role_id": str(role_id)},
        )

    async def remove_role_from_member(
        self, server_id: int, user_id: int, role_id: int
    ) -> None:
        """``DELETE /api/servers/{id}/members/{userID}/roles/{roleID}``"""
        await self.delete(
            f"/api/servers/{server_id}/members/{user_id}/roles/{role_id}"
        )

    async def get_members_with_roles(self, server_id: int) -> list[dict]:
        """``GET /api/servers/{id}/members-with-roles`` — roster joined with roles."""
        return await self.get(f"/api/servers/{server_id}/members-with-roles")
