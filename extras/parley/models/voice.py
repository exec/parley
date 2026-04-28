"""
parley.models.voice
===================
Voice control-plane models.

.. note::
    This client only handles the **parley-side control plane** for voice:
    obtaining a LiveKit JWT, registering presence (join/leave), listing
    participants, force-mute / kick, and DM-call ringing.

    Actual audio I/O (publishing/subscribing audio tracks, encoding, jitter
    buffering) is **out of scope** for this library. To participate in
    audio, pass the :class:`VoiceToken` returned by
    :meth:`~parley.http.HTTPClient.get_voice_token` to the official
    `livekit` Python SDK::

        token = await client.http.get_voice_token("s:42")
        # pip install livekit
        from livekit import rtc
        room = rtc.Room()
        await room.connect(token.url, token.token)
"""

from __future__ import annotations

from typing import Any, Optional

from ..utils import snowflake_to_int_or_none

__all__ = [
    "VoiceToken",
    "VoiceParticipant",
    "Ring",
    "ActiveCalls",
]


class VoiceToken:
    """
    A LiveKit access token + server URL.

    Attributes
    ----------
    token:
        Signed LiveKit JWT (valid ~6 hours).
    url:
        WebSocket URL of the LiveKit SFU.
    """

    __slots__ = ("token", "url")

    def __init__(self, *, token: str, url: str) -> None:
        self.token = token
        self.url = url

    @classmethod
    def _from_data(cls, data: dict) -> "VoiceToken":
        return cls(token=data.get("token", ""), url=data.get("url", ""))

    def __repr__(self) -> str:
        return f"<VoiceToken url={self.url!r}>"


class VoiceParticipant:
    """
    A user currently present in a voice virtual channel.

    Attributes
    ----------
    user_id:
        Integer snowflake of the participant.
    username:
        Display name (falls back to username if no display name is set).
    avatar_url:
        Avatar URL of the participant, or empty string.
    """

    __slots__ = ("user_id", "username", "avatar_url")

    def __init__(self, *, user_id: int, username: str, avatar_url: str = "") -> None:
        self.user_id = user_id
        self.username = username
        self.avatar_url = avatar_url

    @classmethod
    def _from_data(cls, data: dict) -> "VoiceParticipant":
        return cls(
            user_id=int(data.get("user_id", 0) or 0),
            username=data.get("username", "") or "",
            avatar_url=data.get("avatar_url", "") or "",
        )

    def __repr__(self) -> str:
        return f"<VoiceParticipant user_id={self.user_id} username={self.username!r}>"


class Ring:
    """
    An active 1:1 DM ring (incoming or outgoing call invite).

    Attributes
    ----------
    ring_id:
        Server-assigned ring identifier (used to accept / decline / cancel).
    dm_channel_id:
        DM channel the ring is bound to.
    caller_id:
        User ID of the caller.
    target_id:
        User ID of the ring target (callee).
    caller_username:
        Caller's username, if provided.
    caller_display_name:
        Caller's display name, if provided.
    caller_avatar_url:
        Caller's avatar URL, if provided.
    """

    __slots__ = (
        "ring_id",
        "dm_channel_id",
        "caller_id",
        "target_id",
        "caller_username",
        "caller_display_name",
        "caller_avatar_url",
    )

    def __init__(
        self,
        *,
        ring_id: str,
        dm_channel_id: Optional[int],
        caller_id: Optional[int],
        target_id: Optional[int],
        caller_username: str = "",
        caller_display_name: str = "",
        caller_avatar_url: str = "",
    ) -> None:
        self.ring_id = ring_id
        self.dm_channel_id = dm_channel_id
        self.caller_id = caller_id
        self.target_id = target_id
        self.caller_username = caller_username
        self.caller_display_name = caller_display_name
        self.caller_avatar_url = caller_avatar_url

    @classmethod
    def _from_data(cls, data: dict) -> "Ring":
        return cls(
            ring_id=str(data.get("ring_id") or data.get("id") or ""),
            dm_channel_id=snowflake_to_int_or_none(data.get("dm_channel_id")),
            caller_id=snowflake_to_int_or_none(data.get("caller_id")),
            target_id=snowflake_to_int_or_none(data.get("target_id")),
            caller_username=data.get("caller_username", "") or "",
            caller_display_name=data.get("caller_display_name", "") or "",
            caller_avatar_url=data.get("caller_avatar_url", "") or "",
        )

    def __repr__(self) -> str:
        return (
            f"<Ring ring_id={self.ring_id!r} dm_channel_id={self.dm_channel_id} "
            f"caller_id={self.caller_id}>"
        )


class ActiveCalls:
    """
    Snapshot returned by ``GET /api/calls/active``.

    Attributes
    ----------
    rings:
        Open rings targeting the current user.
    in_call:
        List of ``(dm_channel_id, participant_count)`` tuples for any DMs the
        user is a member of that currently have at least one participant in
        a live call.
    """

    __slots__ = ("rings", "in_call")

    def __init__(
        self,
        *,
        rings: list[Ring],
        in_call: list[tuple[int, int]],
    ) -> None:
        self.rings = rings
        self.in_call = in_call

    @classmethod
    def _from_data(cls, data: dict) -> "ActiveCalls":
        rings_raw = data.get("rings") or []
        rings = [Ring._from_data(r) for r in rings_raw]
        in_call_raw = data.get("in_call") or []
        in_call: list[tuple[int, int]] = []
        for entry in in_call_raw:
            dm_id = snowflake_to_int_or_none(entry.get("dm_channel_id"))
            count = int(entry.get("participant_count") or 0)
            if dm_id is not None:
                in_call.append((dm_id, count))
        return cls(rings=rings, in_call=in_call)

    def __repr__(self) -> str:
        return f"<ActiveCalls rings={len(self.rings)} in_call={len(self.in_call)}>"
