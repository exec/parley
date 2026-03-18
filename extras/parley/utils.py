"""
parley.utils
============
Miscellaneous helpers used throughout the library.

Includes:
- :data:`MISSING` sentinel for distinguishing "not provided" from ``None``.
- Snowflake helpers (Parley uses integer IDs).
- Mention parsing utilities.
"""

from __future__ import annotations

import re
from datetime import datetime, timezone
from typing import Any


__all__ = [
    "MISSING",
    "MissingSentinel",
    "snowflake_time",
    "parse_mentions",
    "strip_mention",
    "format_mention",
]

# ---------------------------------------------------------------------------
# MISSING sentinel
# ---------------------------------------------------------------------------


class MissingSentinel:
    """
    Singleton sentinel used to distinguish "argument not passed" from ``None``.

    Example::

        def edit(self, *, name=MISSING):
            if name is not MISSING:
                payload["name"] = name
    """

    _instance: "MissingSentinel | None" = None

    def __new__(cls) -> "MissingSentinel":
        if cls._instance is None:
            cls._instance = super().__new__(cls)
        return cls._instance

    def __repr__(self) -> str:
        return "<MISSING>"

    def __bool__(self) -> bool:
        return False


MISSING: Any = MissingSentinel()
"""Sentinel value used to represent an argument that was not provided."""


# ---------------------------------------------------------------------------
# Snowflake helpers
# ---------------------------------------------------------------------------

# Parley epoch: Unix epoch (ms).  If the platform uses a custom epoch this
# constant can be adjusted without changing downstream code.
PARLEY_EPOCH_MS: int = 0  # standard Unix epoch


def snowflake_time(snowflake_id: int) -> datetime:
    """Convert a Parley snowflake ID to a UTC :class:`datetime`.

    Parameters
    ----------
    snowflake_id:
        An integer snowflake ID.

    Returns
    -------
    :class:`datetime`
        The UTC timestamp encoded in the snowflake, timezone-aware.
    """
    # Top 42 bits are milliseconds since epoch.
    timestamp_ms = (snowflake_id >> 22) + PARLEY_EPOCH_MS
    return datetime.fromtimestamp(timestamp_ms / 1000.0, tz=timezone.utc)


def snowflake_to_int(value: Any) -> int:
    """Coerce a snowflake value (str or int) to ``int``."""
    if value is None:
        raise ValueError("snowflake_to_int received None")
    return int(value)


def snowflake_to_int_or_none(value: Any) -> int | None:
    """Coerce a snowflake to ``int``, returning ``None`` if falsy."""
    if value is None or value == "":
        return None
    try:
        return int(value)
    except (TypeError, ValueError):
        return None


# ---------------------------------------------------------------------------
# Mention helpers
# ---------------------------------------------------------------------------

_MENTION_RE = re.compile(r"<@(\d+)>")
_CHANNEL_MENTION_RE = re.compile(r"<#(\d+)>")


def parse_mentions(content: str) -> list[int]:
    """Return a list of user IDs mentioned in *content*.

    Parameters
    ----------
    content:
        A message string potentially containing ``<@123>`` mentions.

    Returns
    -------
    list[int]
        Unique user IDs found in the string, in order of appearance.
    """
    seen: list[int] = []
    for m in _MENTION_RE.finditer(content):
        uid = int(m.group(1))
        if uid not in seen:
            seen.append(uid)
    return seen


def parse_channel_mentions(content: str) -> list[int]:
    """Return a list of channel IDs mentioned with ``<#id>`` in *content*."""
    seen: list[int] = []
    for m in _CHANNEL_MENTION_RE.finditer(content):
        cid = int(m.group(1))
        if cid not in seen:
            seen.append(cid)
    return seen


def strip_mention(content: str, user_id: int | str) -> str:
    """Remove ``<@user_id>`` occurrences from *content*.

    Parameters
    ----------
    content:
        The message text.
    user_id:
        The user whose mention tags should be stripped.

    Returns
    -------
    str
        Content with the specific mention(s) removed and whitespace normalised.
    """
    uid = str(user_id)
    cleaned = _MENTION_RE.sub(
        lambda m: "" if m.group(1) == uid else m.group(0),
        content,
    )
    return " ".join(cleaned.split())


def format_mention(user_id: int | str) -> str:
    """Return the mention string ``<@user_id>`` for the given ID."""
    return f"<@{user_id}>"
