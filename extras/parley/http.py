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

    # ==================================================================
    # ===== Uploads / Passkey / GDPR / Overwrites / Messages =====
    # ==================================================================

    # ------------------------------------------------------------------
    # Typed upload wrappers
    # ------------------------------------------------------------------
    #
    # The server exposes a single ``POST /api/upload`` endpoint that returns
    # the public CDN URL of the stored object. Setting that URL on a profile,
    # banner, or server icon is a separate PATCH/PUT against the relevant
    # resource. The wrappers below upload the file and then issue that PATCH
    # in one call so callers don't have to wire two requests together.

    async def upload_avatar(self, file_bytes: bytes, filename: str) -> str:
        """Upload an image and set it as the authenticated user's avatar.

        Returns
        -------
        str
            The public URL now stored as ``avatar_url``.
        """
        url = await self.upload_file(file_bytes, filename)
        await self.request("PATCH", "/api/users/me", json={"avatar_url": url})
        return url

    async def upload_banner(self, file_bytes: bytes, filename: str) -> str:
        """Upload an image and set it as the authenticated user's profile banner.

        Returns
        -------
        str
            The public URL now stored as ``banner_url``.
        """
        url = await self.upload_file(file_bytes, filename)
        await self.request("PATCH", "/api/users/me", json={"banner_url": url})
        return url

    async def upload_server_icon(
        self, server_id: int, file_bytes: bytes, filename: str
    ) -> str:
        """Upload an image and set it as the icon of *server_id*.

        Returns
        -------
        str
            The public URL now stored as the server's ``icon_url``.
        """
        url = await self.upload_file(file_bytes, filename)
        await self.put(f"/api/servers/{server_id}", {"icon_url": url})
        return url

    # ------------------------------------------------------------------
    # Passkeys
    # ------------------------------------------------------------------
    #
    # Note on the LOGIN ceremony: ``/api/auth/passkey/login/begin`` and
    # ``/api/auth/passkey/login/finish`` are *unauthenticated* endpoints —
    # they execute before a session token exists. The :class:`HTTPClient`
    # is constructed with a bearer token in the Authorization header, so it
    # is not the right transport for those routes. Library users who need
    # WebAuthn login should drive a fresh :class:`httpx.AsyncClient` against
    # the two endpoints, then construct an :class:`HTTPClient` with the
    # token returned by ``login/finish``.
    #
    # The register ceremony below is authenticated (you must already be
    # signed in to add a passkey to your account) and therefore fits cleanly
    # on this client.

    async def passkey_register_begin(self) -> dict:
        """``POST /api/auth/passkey/register/begin``.

        Returns
        -------
        dict
            ``{"options": <PublicKeyCredentialCreationOptions>, "session_id": str}``.
            The ``options`` blob is opaque WebAuthn data — feed it to a
            browser-side ``navigator.credentials.create()`` call and pass
            the resulting credential back to :meth:`passkey_register_finish`.
        """
        return await self.post("/api/auth/passkey/register/begin")

    async def passkey_register_finish(
        self, session_id: str, name: str, credential: dict
    ) -> None:
        """``POST /api/auth/passkey/register/finish`` — complete registration.

        Parameters
        ----------
        session_id:
            The ``session_id`` returned by :meth:`passkey_register_begin`.
        name:
            User-supplied label for the new passkey.
        credential:
            The ``PublicKeyCredential`` JSON produced by the browser.
        """
        await self.post(
            "/api/auth/passkey/register/finish",
            {"session_id": session_id, "name": name, "credential": credential},
        )

    async def list_passkeys(self) -> list[dict]:
        """``GET /api/auth/passkeys`` — passkeys registered to the current user."""
        return await self.get("/api/auth/passkeys")

    async def delete_passkey(self, passkey_id: str) -> None:
        """``DELETE /api/auth/passkeys/{id}``"""
        await self.delete(f"/api/auth/passkeys/{passkey_id}")

    async def rename_passkey(self, passkey_id: str, name: str) -> None:
        """``PUT /api/auth/passkeys/{id}`` — rename a registered passkey."""
        await self.put(f"/api/auth/passkeys/{passkey_id}", {"name": name})

    async def remove_password(self) -> None:
        """``DELETE /api/auth/password`` — clear the account password.

        Only succeeds if at least one passkey is registered. Backend
        enforces this atomically so the account cannot be left with neither
        credential after a concurrent passkey deletion.
        """
        await self.delete("/api/auth/password")

    # ------------------------------------------------------------------
    # GDPR — self-serve account deletion + data export
    # ------------------------------------------------------------------

    async def export_account_data(self) -> dict:
        """``GET /api/me/export`` — full GDPR-portability envelope.

        Returns the entire export payload as a parsed dict (profile, messages,
        DMs, friends, audit, themes, etc.). The response can be sizeable —
        callers that intend to persist it should write it to disk rather than
        keeping the dict in memory.
        """
        return await self.get("/api/me/export")

    async def delete_account(self, confirm_username: str) -> None:
        """``DELETE /api/me`` — irreversibly delete the authenticated account.

        WARNING — this is destructive and cannot be undone. The backend
        purges the user's profile, messages, DMs, friends, themes, sessions,
        and uploads. Bots and servers owned solely by this user are removed;
        servers or group DMs that still have other members will respond with
        HTTP 409 (raised as :class:`~parley.errors.HTTPError` here) and you
        must transfer ownership before retrying.

        Parameters
        ----------
        confirm_username:
            Must exactly match the authenticated user's current username.
            The server rejects the request with 400 if it doesn't.
        """
        await self.request(
            "DELETE", "/api/me", json={"confirm_username": confirm_username}
        )

    # ------------------------------------------------------------------
    # Channel permission overwrites
    # ------------------------------------------------------------------
    #
    # ``target_type`` follows the backend convention: ``0`` for a role
    # overwrite, ``1`` for a user overwrite. The :class:`~parley.models.Overwrite`
    # model exposes role_id/user_id convenience properties on top of these.

    async def get_channel_overwrites(self, channel_id: int) -> list[dict]:
        """``GET /api/channels/{id}/overwrites`` — list permission overwrites.

        Backend gates this on ViewChannel — callers that cannot see the
        channel will receive 404, not an empty list.
        """
        return await self.get(f"/api/channels/{channel_id}/overwrites")

    async def upsert_channel_overwrite(
        self,
        channel_id: int,
        *,
        target_type: int,
        target_id: int,
        allow: int = 0,
        deny: int = 0,
    ) -> dict:
        """``PUT /api/channels/{id}/overwrites`` — create or update an overwrite.

        Parameters
        ----------
        channel_id:
            Channel the overwrite scopes to.
        target_type:
            ``0`` for a role, ``1`` for a user.
        target_id:
            Snowflake ID of the role or user being overridden.
        allow, deny:
            Bit-flag :class:`~parley.enums.Permissions` values granted/revoked
            in this channel. Conflicting bits are resolved by the backend
            (allow wins).
        """
        return await self.put(
            f"/api/channels/{channel_id}/overwrites",
            {
                "target_type": target_type,
                "target_id": str(target_id),
                "allow": allow,
                "deny": deny,
            },
        )

    async def delete_channel_overwrite(
        self, channel_id: int, overwrite_id: int
    ) -> None:
        """``DELETE /api/channels/{id}/overwrites/{overwriteId}``"""
        await self.delete(
            f"/api/channels/{channel_id}/overwrites/{overwrite_id}"
        )

    async def get_my_channel_permissions(self, channel_id: int) -> dict:
        """``GET /api/channels/{id}/my-permissions`` — effective bits for the actor."""
        return await self.get(f"/api/channels/{channel_id}/my-permissions")

    # ------------------------------------------------------------------
    # Message extras — search, forward, history, reactions
    # ------------------------------------------------------------------

    async def search_messages(
        self,
        server_id: int,
        *,
        q: str,
        from_user_id: Optional[int] = None,
        in_channel_id: Optional[int] = None,
        limit: int = 25,
        before: Optional[int] = None,
    ) -> list[dict]:
        """``GET /api/servers/{id}/messages/search``.

        Parameters
        ----------
        server_id:
            Server to search within.
        q:
            Query string (server-side ILIKE, rate limited at 20/min).
        from_user_id:
            Restrict results to messages by this author.
        in_channel_id:
            Restrict results to a single channel.
        limit:
            Max results (1–50, default 25).
        before:
            Snowflake ID — return messages older than this.
        """
        # `from` and `in` are Python keywords, so build the params dict
        # manually and route through ``request`` (which strips None values).
        params: dict = {"q": q, "limit": limit}
        if from_user_id is not None:
            params["from"] = str(from_user_id)
        if in_channel_id is not None:
            params["in"] = str(in_channel_id)
        if before is not None:
            params["before"] = before
        return await self.request(
            "GET",
            f"/api/servers/{server_id}/messages/search",
            params=params,
        )

    async def forward_message(
        self,
        target_channel_id: int,
        *,
        source_message_id: int,
        source_channel_id: Optional[int] = None,
    ) -> dict:
        """``POST /api/channels/{channelID}/forward`` — forward into a server channel.

        Parameters
        ----------
        target_channel_id:
            Channel to deliver the forwarded snapshot into.
        source_message_id:
            Snowflake ID of the message being forwarded. The server
            re-resolves the snapshot (author, content, channel/server names,
            timestamp) from the source row — caller-supplied content is
            ignored.
        source_channel_id:
            Optional source channel ID for disambiguation when the source
            is a DM/group DM.
        """
        body: dict = {"message_id": str(source_message_id)}
        if source_channel_id is not None:
            body["channel_id"] = str(source_channel_id)
        return await self.post(
            f"/api/channels/{target_channel_id}/forward", body
        )

    async def forward_message_to_dm(
        self,
        target_dm_channel_id: int,
        *,
        source_message_id: int,
        source_channel_id: Optional[int] = None,
    ) -> dict:
        """``POST /api/dms/{id}/forward`` — forward into a DM or group DM."""
        body: dict = {"message_id": str(source_message_id)}
        if source_channel_id is not None:
            body["channel_id"] = str(source_channel_id)
        return await self.post(
            f"/api/dms/{target_dm_channel_id}/forward", body
        )

    async def get_message_versions(self, message_id: int) -> list[dict]:
        """``GET /api/messages/{id}/versions`` — full edit history of a message."""
        return await self.get(f"/api/messages/{message_id}/versions")

    async def get_channel_pins(self, channel_id: int) -> list[dict]:
        """``GET /api/channels/{channelID}/pins`` — pinned messages in a channel."""
        return await self.get(f"/api/channels/{channel_id}/pins")

    async def pin_message(self, channel_id: int, message_id: int) -> None:
        """``POST /api/channels/{channelID}/pins/{messageID}``"""
        await self.post(f"/api/channels/{channel_id}/pins/{message_id}")

    async def unpin_message(self, channel_id: int, message_id: int) -> None:
        """``DELETE /api/channels/{channelID}/pins/{messageID}``"""
        await self.delete(f"/api/channels/{channel_id}/pins/{message_id}")

    async def toggle_reaction(self, message_id: int, emoji: str) -> None:
        """``POST /api/messages/{id}/reactions`` — toggle a reaction.

        The backend has a single toggle endpoint: calling this when the
        emoji is absent adds it, calling it again removes it. There is no
        separate "remove reaction" route. To list the current reactions on
        a message, fetch the message itself — reactions are returned inline
        on the message payload.
        """
        await self.post(
            f"/api/messages/{message_id}/reactions", {"emoji": emoji}
        )

    async def toggle_dm_reaction(
        self, dm_channel_id: int, message_id: int, emoji: str
    ) -> None:
        """``POST /api/dms/{id}/messages/{messageId}/reactions`` — DM toggle."""
        await self.post(
            f"/api/dms/{dm_channel_id}/messages/{message_id}/reactions",
            {"emoji": emoji},
        )
