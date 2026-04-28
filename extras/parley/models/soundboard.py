"""
parley.models.soundboard
========================
Soundboard sound model.
"""

from __future__ import annotations

from datetime import datetime
from typing import Any, Optional

from ..utils import snowflake_to_int, snowflake_to_int_or_none

__all__ = ["Sound"]


def _parse_iso8601(value: Any) -> Optional[datetime]:
    if not value:
        return None
    if isinstance(value, datetime):
        return value
    s = str(value)
    # Tolerate trailing 'Z' that older Python's fromisoformat rejects.
    if s.endswith("Z"):
        s = s[:-1] + "+00:00"
    try:
        return datetime.fromisoformat(s)
    except ValueError:
        return None


class Sound:
    """
    A soundboard sound entry.

    Attributes
    ----------
    id:
        Snowflake ID of the sound.
    server_id:
        Server the sound belongs to.
    uploader_id:
        User who uploaded the sound.
    name:
        Sound display name.
    emoji:
        Optional emoji shown alongside the sound.
    file_url:
        Public URL to the audio file.
    server_name:
        Set when the sound is returned from the cross-server listing endpoint.
    created_at:
        Upload timestamp (UTC).
    """

    __slots__ = (
        "id",
        "server_id",
        "uploader_id",
        "name",
        "emoji",
        "file_url",
        "server_name",
        "created_at",
        "_state",
    )

    def __init__(
        self,
        *,
        id: int,
        server_id: int,
        uploader_id: Optional[int],
        name: str,
        emoji: str,
        file_url: str,
        server_name: str = "",
        created_at: Optional[datetime] = None,
        state: Optional[Any] = None,
    ) -> None:
        self.id = id
        self.server_id = server_id
        self.uploader_id = uploader_id
        self.name = name
        self.emoji = emoji
        self.file_url = file_url
        self.server_name = server_name
        self.created_at = created_at
        self._state: Optional[Any] = state

    @classmethod
    def _from_data(cls, data: dict, state: Optional[Any] = None) -> "Sound":
        return cls(
            id=snowflake_to_int(data["id"]),
            server_id=snowflake_to_int(data.get("server_id", 0) or 0),
            uploader_id=snowflake_to_int_or_none(data.get("uploader_id")),
            name=data.get("name", "") or "",
            emoji=data.get("emoji", "") or "",
            file_url=data.get("file_url", "") or "",
            server_name=data.get("server_name", "") or "",
            created_at=_parse_iso8601(data.get("created_at")),
            state=state,
        )

    def __repr__(self) -> str:
        return f"<Sound id={self.id} name={self.name!r} server_id={self.server_id}>"

    def __eq__(self, other: object) -> bool:
        return isinstance(other, Sound) and self.id == other.id

    def __hash__(self) -> int:
        return hash(self.id)
