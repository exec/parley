"""
parley.state
============
:class:`ConnectionState` — the single source of truth for all cached data.

The state layer sits between the gateway/HTTP and the client.  It:

1. Holds caches for servers, channels, members, users, and messages.
2. Receives raw gateway events from :class:`~parley.gateway.GatewayClient`.
3. Parses raw dicts into model objects, updating the cache.
4. Calls ``client.dispatch(event_name, *args)`` so application code runs.

Nothing in this module should be imported directly by application code.
"""

from __future__ import annotations

import logging
from collections import deque
from typing import TYPE_CHECKING, Any, Callable, Optional

from .models.channel import TextChannel, channel_from_data
from .models.dm import DmChannel
from .models.invite import Invite
from .models.member import Member
from .models.message import DmMessage, Message
from .models.role import Role
from .models.server import Server
from .models.user import PublicUser, User

if TYPE_CHECKING:
    from .http import HTTPClient

__all__ = ["ConnectionState"]

log = logging.getLogger("parley.state")

# Maximum number of messages to keep in the LRU-style deque.
_MAX_MESSAGES = 1000


class ConnectionState:
    """
    In-memory cache and event-processing layer.

    Parameters
    ----------
    http:
        The :class:`~parley.http.HTTPClient` to attach to all model objects.
    dispatch:
        Callable used to fire events into application code.  Signature:
        ``dispatch(event_name: str, *args) -> None``.
    """

    def __init__(self, http: "HTTPClient", dispatch: Callable) -> None:
        self.http = http
        self._dispatch = dispatch

        # Lazy-set after login / get_me
        self.user: Optional[User] = None

        # Caches
        self.servers: dict[int, Server] = {}
        self.channels: dict[int, Any] = {}  # Channel subclasses
        self.members: dict[tuple[int, int], Member] = {}  # (server_id, user_id)
        self.users: dict[int, PublicUser] = {}
        self.messages: deque[Message] = deque(maxlen=_MAX_MESSAGES)
        self._message_map: dict[int, Message] = {}  # id → Message for O(1) lookup
        self.dm_channels: dict[int, DmChannel] = {}
        self.roles: dict[int, Role] = {}  # role_id → Role

        # Back-reference so models can call gateway (set by Client)
        self.gateway: Any = None

    # ------------------------------------------------------------------
    # Cache helpers
    # ------------------------------------------------------------------

    def _add_message(self, msg: Message) -> None:
        """Add a message to the rolling cache, evicting the oldest if full."""
        if len(self.messages) == _MAX_MESSAGES:
            old = self.messages[0]  # will be evicted
            self._message_map.pop(old.id, None)
        self.messages.append(msg)
        self._message_map[msg.id] = msg

    def get_message(self, message_id: int) -> Optional[Message]:
        """Look up a cached :class:`~parley.models.message.Message` by ID."""
        return self._message_map.get(message_id)

    def get_channel(self, channel_id: int) -> Optional[Any]:
        """Look up a cached channel by ID."""
        return self.channels.get(channel_id)

    def get_server(self, server_id: int) -> Optional[Server]:
        """Look up a cached server by ID."""
        return self.servers.get(server_id)

    def get_member(self, server_id: int, user_id: int) -> Optional[Member]:
        """Look up a cached member by (server_id, user_id)."""
        return self.members.get((server_id, user_id))

    # ------------------------------------------------------------------
    # Gateway hooks
    # ------------------------------------------------------------------

    async def _on_gateway_connected(self) -> None:
        """Called by :class:`~parley.gateway.GatewayClient` on each connection.

        Populates the server/channel cache and auto-subscribes to text channels.
        """
        try:
            me_data = await self.http.get_me()
            self.user = User._from_data(me_data, self)
            log.info("Logged in as %s (id=%d)", self.user.display, self.user.id)
        except Exception:
            log.exception("Failed to fetch own profile on connect")

        await self._populate_servers()
        await self._dispatch_event("READY", {})

    async def _populate_servers(self) -> None:
        """Fetch servers + channels, populate cache, subscribe to text channels."""
        try:
            server_list = await self.http.get_servers()
        except Exception:
            log.exception("Failed to fetch server list")
            return

        for s_data in server_list:
            server = Server._from_data(s_data, self)
            self.servers[server.id] = server

            try:
                ch_list = await self.http.get_server_channels(server.id)
            except Exception:
                log.warning("Could not fetch channels for server %d", server.id)
                continue

            for c_data in ch_list:
                ch = channel_from_data(c_data, self)
                self.channels[ch.id] = ch
                if isinstance(ch, TextChannel) and self.gateway is not None:
                    try:
                        await self.gateway.subscribe(ch.id)
                    except Exception:
                        log.warning("Failed to subscribe to channel %d", ch.id)

        log.info(
            "Cached %d server(s), %d channel(s)",
            len(self.servers),
            len(self.channels),
        )

    async def _process_event(self, event_type: str, payload: dict) -> None:
        """Parse a raw gateway event and dispatch it to application code.

        Parameters
        ----------
        event_type:
            The ``"type"`` string from the WS frame (e.g. ``MESSAGE_CREATE``).
        payload:
            The ``"payload"`` dict from the WS frame.
        """
        handler = _EVENT_HANDLERS.get(event_type)
        if handler is not None:
            try:
                await handler(self, payload)
            except Exception:
                log.exception("Error processing event %s", event_type)
        else:
            # Dispatch unknown events raw so advanced users can handle them.
            await self._dispatch_event(event_type, payload)

    async def _dispatch_event(self, event_type: str, payload: dict) -> None:
        """Normalise event name and call the client dispatch function."""
        name = event_type.lower()
        self._dispatch(name, payload)

    # ------------------------------------------------------------------
    # Event handlers  (one per gateway event type)
    # ------------------------------------------------------------------

    async def _handle_message_create(self, payload: dict) -> None:
        msg = Message._from_data(payload, self)
        self._add_message(msg)
        self._dispatch("message_create", msg)

    async def _handle_message_update(self, payload: dict) -> None:
        msg_id = int(payload.get("id", 0))
        cached = self._message_map.get(msg_id)
        if cached is not None:
            cached._update(payload)
            msg = cached
        else:
            msg = Message._from_data(payload, self)
            self._add_message(msg)
        self._dispatch("message_update", msg)

    async def _handle_message_delete(self, payload: dict) -> None:
        msg_id = int(payload.get("id", 0))
        cached = self._message_map.pop(msg_id, None)
        self._dispatch("message_delete", payload)

    async def _handle_channel_create(self, payload: dict) -> None:
        ch = channel_from_data(payload, self)
        self.channels[ch.id] = ch
        if isinstance(ch, TextChannel) and self.gateway is not None:
            try:
                await self.gateway.subscribe(ch.id)
            except Exception:
                pass
        self._dispatch("channel_create", ch)

    async def _handle_channel_update(self, payload: dict) -> None:
        ch_id = int(payload.get("id", 0))
        cached = self.channels.get(ch_id)
        if cached is not None:
            cached._update(payload)
            self._dispatch("channel_update", cached)
        else:
            ch = channel_from_data(payload, self)
            self.channels[ch.id] = ch
            self._dispatch("channel_update", ch)

    async def _handle_channel_delete(self, payload: dict) -> None:
        ch_id = int(payload.get("id", 0))
        self.channels.pop(ch_id, None)
        self._dispatch("channel_delete", payload)

    async def _handle_server_update(self, payload: dict) -> None:
        srv_id = int(payload.get("id", 0))
        cached = self.servers.get(srv_id)
        if cached is not None:
            cached._update(payload)
            self._dispatch("server_update", cached)
        else:
            server = Server._from_data(payload, self)
            self.servers[server.id] = server
            self._dispatch("server_update", server)

    async def _handle_server_delete(self, payload: dict) -> None:
        srv_id = int(payload.get("id", 0))
        self.servers.pop(srv_id, None)
        self._dispatch("server_delete", payload)

    async def _handle_member_join(self, payload: dict) -> None:
        member = Member._from_data(payload, self)
        self.members[(member.server_id, member.user_id)] = member
        self._dispatch("server_member_join", member)

    async def _handle_member_leave(self, payload: dict) -> None:
        member = Member._from_data(payload, self)
        self.members.pop((member.server_id, member.user_id), None)
        self._dispatch("server_member_leave", member)

    async def _handle_member_kick(self, payload: dict) -> None:
        member = Member._from_data(payload, self)
        self.members.pop((member.server_id, member.user_id), None)
        self._dispatch("server_member_kick", member)

    async def _handle_member_ban(self, payload: dict) -> None:
        member = Member._from_data(payload, self)
        self.members.pop((member.server_id, member.user_id), None)
        self._dispatch("server_member_ban", member)

    async def _handle_member_role_update(self, payload: dict) -> None:
        server_id = int(payload.get("server_id", 0))
        user_id = int(payload.get("user_id", 0))
        cached = self.members.get((server_id, user_id))
        if cached is not None:
            cached._update(payload)
        self._dispatch("member_role_update", payload)

    async def _handle_role_update(self, payload: dict) -> None:
        role_id = int(payload.get("id", 0))
        cached = self.roles.get(role_id)
        if cached is not None:
            cached._update(payload)
            self._dispatch("role_update", cached)
        else:
            role = Role._from_data(payload, self)
            self.roles[role.id] = role
            self._dispatch("role_update", role)

    async def _handle_role_delete(self, payload: dict) -> None:
        role_id = int(payload.get("id", 0))
        self.roles.pop(role_id, None)
        self._dispatch("role_delete", payload)

    async def _handle_user_update(self, payload: dict) -> None:
        user_id = int(payload.get("user_id", 0))
        cached = self.users.get(user_id)
        if cached is not None:
            if "username" in payload:
                cached.username = payload["username"]
            if "display_name" in payload:
                cached.display_name = payload["display_name"] or cached.username
            if "avatar_url" in payload:
                cached.avatar_url = payload["avatar_url"] or ""
            if "bio" in payload:
                cached.bio = payload["bio"] or ""
        # Also update self if it's us
        if self.user and self.user.id == user_id:
            if "username" in payload:
                self.user.username = payload["username"]
            if "display_name" in payload:
                self.user.display_name = payload["display_name"] or self.user.username
            if "avatar_url" in payload:
                self.user.avatar_url = payload["avatar_url"] or ""
        self._dispatch("user_update", payload)

    async def _handle_presence_snapshot(self, payload: dict) -> None:
        self._dispatch("presence_snapshot", payload)

    async def _handle_user_online(self, payload: dict) -> None:
        self._dispatch("user_online", payload)

    async def _handle_user_offline(self, payload: dict) -> None:
        self._dispatch("user_offline", payload)

    async def _handle_user_typing(self, payload: dict) -> None:
        self._dispatch("user_typing", payload)

    async def _handle_reaction_add(self, payload: dict) -> None:
        self._dispatch("reaction_add", payload)

    async def _handle_reaction_remove(self, payload: dict) -> None:
        self._dispatch("reaction_remove", payload)

    async def _handle_voice_state_update(self, payload: dict) -> None:
        self._dispatch("voice_state_update", payload)

    async def _handle_bot_status_update(self, payload: dict) -> None:
        self._dispatch("bot_status_update", payload)


# ------------------------------------------------------------------
# Event handler dispatch table
# ------------------------------------------------------------------

_EVENT_HANDLERS: dict[str, Any] = {
    "MESSAGE_CREATE": ConnectionState._handle_message_create,
    "MESSAGE_UPDATE": ConnectionState._handle_message_update,
    "MESSAGE_DELETE": ConnectionState._handle_message_delete,
    "CHANNEL_CREATE": ConnectionState._handle_channel_create,
    "CHANNEL_UPDATE": ConnectionState._handle_channel_update,
    "CHANNEL_DELETE": ConnectionState._handle_channel_delete,
    "SERVER_UPDATE": ConnectionState._handle_server_update,
    "SERVER_DELETE": ConnectionState._handle_server_delete,
    "SERVER_MEMBER_JOIN": ConnectionState._handle_member_join,
    "SERVER_MEMBER_LEAVE": ConnectionState._handle_member_leave,
    "SERVER_MEMBER_KICK": ConnectionState._handle_member_kick,
    "SERVER_MEMBER_BAN": ConnectionState._handle_member_ban,
    "MEMBER_ROLE_UPDATE": ConnectionState._handle_member_role_update,
    "ROLE_UPDATE": ConnectionState._handle_role_update,
    "ROLE_DELETE": ConnectionState._handle_role_delete,
    "USER_UPDATE": ConnectionState._handle_user_update,
    "PRESENCE_SNAPSHOT": ConnectionState._handle_presence_snapshot,
    "USER_ONLINE": ConnectionState._handle_user_online,
    "USER_OFFLINE": ConnectionState._handle_user_offline,
    "USER_TYPING": ConnectionState._handle_user_typing,
    "REACTION_ADD": ConnectionState._handle_reaction_add,
    "REACTION_REMOVE": ConnectionState._handle_reaction_remove,
    "VOICE_STATE_UPDATE": ConnectionState._handle_voice_state_update,
    "BOT_STATUS_UPDATE": ConnectionState._handle_bot_status_update,
}
