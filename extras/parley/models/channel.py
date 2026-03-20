"""
parley.models.channel
=====================
Channel model hierarchy:

- :class:`Channel`          — base class, not meant to be instantiated directly.
- :class:`TextChannel`      — text channel (``channel_type == 0``).
- :class:`VoiceChannel`     — voice channel (``channel_type == 1``); voice stubbed.
- :class:`BinChannel`       — binary/file channel (``channel_type == 2``).

Use :func:`channel_from_data` to construct the right subclass from a raw dict.
"""

from __future__ import annotations

import asyncio
from typing import TYPE_CHECKING, Any, Awaitable, Callable, Optional

from ..enums import ChannelType
from ..utils import snowflake_to_int, snowflake_to_int_or_none


class Typing:
    """Async context manager that broadcasts a typing indicator while active.

    Calls *send_fn* immediately on enter, then repeatedly every *interval*
    seconds until the block exits.

    Bot clients use the REST path (longer intervals, server-managed expiry);
    non-Bot clients use the WS TYPING frame path (5-second intervals).
    """

    def __init__(
        self,
        channel_id: int,
        send_fn: "Callable[[], Awaitable[None]]",
        interval: float = 5.0,
    ) -> None:
        self._channel_id = channel_id
        self._send_fn = send_fn
        self._interval = interval
        self._task: Optional[asyncio.Task] = None

    async def _loop(self) -> None:
        try:
            while True:
                await self._send_fn()
                await asyncio.sleep(self._interval)
        except asyncio.CancelledError:
            pass

    async def __aenter__(self) -> "Typing":
        self._task = asyncio.create_task(self._loop())
        return self

    async def __aexit__(self, *_: Any) -> None:
        if self._task:
            self._task.cancel()
            try:
                await self._task
            except asyncio.CancelledError:
                pass
            self._task = None

if TYPE_CHECKING:
    from ..state import ConnectionState
    from ..client import Bot

__all__ = [
    "Channel",
    "TextChannel",
    "VoiceChannel",
    "BinChannel",
    "channel_from_data",
]


class Channel:
    """
    Base class for all server channels.

    Attributes
    ----------
    id:
        Integer snowflake ID.
    server_id:
        ID of the parent :class:`~parley.models.server.Server`.
    name:
        Channel name.
    channel_type:
        :class:`~parley.enums.ChannelType` enum value.
    topic:
        Channel topic/description (may be empty string).
    position:
        Ordering position within the server.
    parent_id:
        ID of the category channel this belongs to, or ``None``.
    """

    __slots__ = (
        "id",
        "server_id",
        "name",
        "channel_type",
        "topic",
        "position",
        "parent_id",
        "_state",
    )

    def __init__(
        self,
        *,
        id: int,
        server_id: int,
        name: str,
        channel_type: ChannelType,
        topic: str,
        position: int,
        parent_id: Optional[int],
        state: Optional[Any] = None,
    ) -> None:
        self.id = id
        self.server_id = server_id
        self.name = name
        self.channel_type = channel_type
        self.topic = topic
        self.position = position
        self.parent_id = parent_id
        self._state: Optional[Any] = state

    @classmethod
    def _from_data(cls, data: dict, state: Optional[Any] = None) -> "Channel":
        return cls(
            id=snowflake_to_int(data["id"]),
            server_id=snowflake_to_int(data.get("server_id", 0) or 0),
            name=data.get("name", ""),
            channel_type=ChannelType(data.get("channel_type", 0)),
            topic=data.get("topic", "") or "",
            position=data.get("position", 0) or 0,
            parent_id=snowflake_to_int_or_none(data.get("parent_id")),
            state=state,
        )

    def _update(self, data: dict) -> None:
        """Apply a partial update from a ``CHANNEL_UPDATE`` WS event."""
        if "name" in data:
            self.name = data["name"]
        if "topic" in data:
            self.topic = data["topic"] or ""
        if "position" in data:
            self.position = data["position"]
        if "parent_id" in data:
            self.parent_id = snowflake_to_int_or_none(data["parent_id"])

    # ------------------------------------------------------------------
    # Async helpers
    # ------------------------------------------------------------------

    async def edit(
        self,
        *,
        name: Optional[str] = None,
        topic: Optional[str] = None,
    ) -> "Channel":
        """Edit this channel.

        Returns
        -------
        :class:`Channel`
            The updated channel.
        """
        data = await self._state.http.edit_channel(self.id, name=name, topic=topic)
        self._update(data)
        return self

    async def delete(self) -> None:
        """Delete this channel."""
        await self._state.http.delete_channel(self.id)

    def __repr__(self) -> str:
        return (
            f"<{type(self).__name__} id={self.id} name={self.name!r} "
            f"server_id={self.server_id}>"
        )

    def __eq__(self, other: object) -> bool:
        return isinstance(other, Channel) and self.id == other.id

    def __hash__(self) -> int:
        return hash(self.id)


class TextChannel(Channel):
    """
    A text channel where members can send messages.

    Provides :meth:`send` and :meth:`fetch_messages` helpers.
    """

    async def send(self, content: str, *, reply_to: Optional[int] = None):
        """Send a message to this channel.

        Parameters
        ----------
        content:
            Text content of the message.
        reply_to:
            Optional message ID to reply to (thread reply).

        Returns
        -------
        :class:`~parley.models.message.Message`
        """
        from .message import Message

        data = await self._state.http.create_message(
            self.id, content, parent_id=reply_to
        )
        return Message._from_data(data, self._state)

    async def fetch_messages(
        self,
        *,
        limit: int = 50,
        before: Optional[int] = None,
    ) -> list:
        """Fetch message history for this channel.

        Parameters
        ----------
        limit:
            Number of messages to return (max 50).
        before:
            Return messages before this message ID.

        Returns
        -------
        list[:class:`~parley.models.message.Message`]
        """
        from .message import Message

        data = await self._state.http.get_messages(self.id, limit=limit, before=before)
        return [Message._from_data(m, self._state) for m in data]

    def typing(self, duration: int = 5) -> Typing:
        """Return a context manager that shows a typing indicator.

        Bot clients use the REST endpoint with server-managed expiry (*duration*
        seconds, clamped to 1-60). Non-Bot clients use the WS TYPING frame path.

        Usage::

            async with channel.typing():
                reply = await slow_ai_call()
            await channel.send(reply)
        """
        client = self._state._client if self._state is not None else None

        # Use name-based check to avoid circular import at runtime.
        # (importing Bot from client.py here would create a cycle.)
        is_bot = (
            client is not None
            and type(client).__name__ in ("Bot", "CommandBot")
        )

        if is_bot:
            # REST path: send once per (duration - 2s buffer), minimum 3s interval.
            interval = float(max(3, duration - 2))
            send_fn = lambda: client.send_typing(self.id, duration)  # type: ignore[union-attr]
        else:
            # WS path: server expires typing in ~5s, resend every 5s.
            interval = 5.0
            state = self._state

            async def ws_send() -> None:
                if state is not None and state.gateway is not None:
                    await state.gateway.send("TYPING", {"channel_id": str(self.id)})

            send_fn = ws_send

        return Typing(self.id, send_fn, interval)

    async def subscribe(self) -> None:
        """Subscribe to real-time events for this channel via the gateway."""
        await self._state.gateway.subscribe(self.id)


class VoiceChannel(Channel):
    """
    A voice channel (LiveKit-backed audio/video).

    Voice functionality is not implemented in this library.
    Calling :meth:`connect` raises :class:`NotImplementedError`.
    """

    async def connect(self) -> None:
        """Connect to this voice channel.

        .. note::
            Voice is powered by LiveKit.  Audio/video connectivity is not
            implemented in the Parley Python library.  You would need to
            integrate directly with the LiveKit SDK.
        """
        raise NotImplementedError(
            "Voice channels use LiveKit for audio/video. "
            "Direct voice connection is not supported by the Parley Python library. "
            "Integrate with the LiveKit Python SDK to connect to voice channels."
        )


class BinChannel(Channel):
    """
    A binary/file-bin channel.

    Messages in bin channels typically contain file attachments.
    Behaves like a :class:`TextChannel` for message sending purposes.
    """

    async def send(self, content: str, *, reply_to: Optional[int] = None):
        """Send a message to this bin channel.

        Returns
        -------
        :class:`~parley.models.message.Message`
        """
        from .message import Message

        data = await self._state.http.create_message(
            self.id, content, parent_id=reply_to
        )
        return Message._from_data(data, self._state)

    async def fetch_messages(
        self,
        *,
        limit: int = 50,
        before: Optional[int] = None,
    ) -> list:
        """Fetch message history for this bin channel.

        Returns
        -------
        list[:class:`~parley.models.message.Message`]
        """
        from .message import Message

        data = await self._state.http.get_messages(self.id, limit=limit, before=before)
        return [Message._from_data(m, self._state) for m in data]


# ------------------------------------------------------------------
# Factory
# ------------------------------------------------------------------

_TYPE_MAP = {
    ChannelType.text: TextChannel,
    ChannelType.voice: VoiceChannel,
    ChannelType.bin: BinChannel,
}


def channel_from_data(data: dict, state: Optional[Any] = None) -> Channel:
    """Construct the appropriate :class:`Channel` subclass from a raw data dict.

    Parameters
    ----------
    data:
        Raw API response dict for a channel.
    state:
        :class:`~parley.state.ConnectionState` reference to attach.

    Returns
    -------
    :class:`Channel`
        A :class:`TextChannel`, :class:`VoiceChannel`, or :class:`BinChannel`.
    """
    ct = ChannelType(data.get("channel_type", 0))
    cls = _TYPE_MAP.get(ct, TextChannel)
    return cls._from_data(data, state)
