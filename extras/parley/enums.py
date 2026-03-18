"""
parley.enums
============
Enumeration types and flag classes used throughout the library.
"""

from __future__ import annotations

from enum import IntEnum, IntFlag


__all__ = [
    "ChannelType",
    "Badges",
    "Permissions",
]


class ChannelType(IntEnum):
    """Type of a server channel."""

    text = 0
    """A standard text channel where members can post messages."""

    voice = 1
    """A voice channel (LiveKit-backed; audio/video only)."""

    bin = 2
    """A binary / file-bin channel."""

    @classmethod
    def _missing_(cls, value: object) -> "ChannelType":  # type: ignore[override]
        # Gracefully fall back for unknown future values.
        return cls.text


class Badges(IntFlag):
    """Bit-flag badges shown on a user profile."""

    none = 0
    staff = 1 << 0
    """Parley platform staff member."""
    verified_bot = 1 << 1
    """Verified bot account."""
    early_supporter = 1 << 2
    """Early supporter of the platform."""
    developer = 1 << 3
    """Registered developer / API user."""
    moderator = 1 << 4
    """Platform-level moderator."""


class Permissions(IntFlag):
    """
    Server/role permission bit-flags.

    Values mirror a typical Discord-like permission model.  Not all may be
    enforced by the server; consult the Parley documentation for which bits
    are in active use.
    """

    none = 0
    view_channels = 1 << 0
    send_messages = 1 << 1
    manage_messages = 1 << 2
    manage_channels = 1 << 3
    manage_server = 1 << 4
    kick_members = 1 << 5
    ban_members = 1 << 6
    manage_roles = 1 << 7
    create_invites = 1 << 8
    administrator = 1 << 9
    """Grants all permissions; overrides all other denies."""
