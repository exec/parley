"""
parley.models.notification
==========================
:class:`Notification` — an entry in the authenticated user's inbox.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any, Optional

from ..utils import snowflake_to_int, snowflake_to_int_or_none

if TYPE_CHECKING:
    from ..state import ConnectionState

__all__ = ["Notification"]


class Notification:
    """
    A notification delivered to the authenticated user's inbox.

    Attributes
    ----------
    id:
        Integer snowflake ID.
    type:
        Notification kind (e.g. ``"mention"``, ``"dm"``, ``"friend_request"``,
        ``"friend_accept"``).
    title:
        Short headline.
    body:
        Body text.
    actor_username:
        Username of the actor that triggered the notification.
    actor_avatar_url:
        Avatar URL of the actor (may be empty string).
    server_id:
        Originating server ID, or ``None`` if not server-scoped.
    channel_id:
        Originating channel ID, or ``None``.
    message_id:
        Originating message ID, or ``None``.
    dm_channel_id:
        Originating DM channel ID, or ``None`` for non-DM notifications.
    read:
        Whether the notification has been marked read.
    created_at:
        ISO-8601 creation timestamp.
    """

    __slots__ = (
        "id",
        "type",
        "title",
        "body",
        "actor_username",
        "actor_avatar_url",
        "server_id",
        "channel_id",
        "message_id",
        "dm_channel_id",
        "read",
        "created_at",
        "_state",
    )

    def __init__(
        self,
        *,
        id: int,
        type: str,
        title: str,
        body: str,
        actor_username: str,
        actor_avatar_url: str,
        server_id: Optional[int],
        channel_id: Optional[int],
        message_id: Optional[int],
        dm_channel_id: Optional[int],
        read: bool,
        created_at: str,
        state: Optional[Any] = None,
    ) -> None:
        self.id = id
        self.type = type
        self.title = title
        self.body = body
        self.actor_username = actor_username
        self.actor_avatar_url = actor_avatar_url
        self.server_id = server_id
        self.channel_id = channel_id
        self.message_id = message_id
        self.dm_channel_id = dm_channel_id
        self.read = read
        self.created_at = created_at
        self._state: Optional[Any] = state

    @classmethod
    def _from_data(cls, data: dict, state: Optional[Any] = None) -> "Notification":
        return cls(
            id=snowflake_to_int(data["id"]),
            type=data.get("type", "") or "",
            title=data.get("title", "") or "",
            body=data.get("body", "") or "",
            actor_username=data.get("actor_username", "") or "",
            actor_avatar_url=data.get("actor_avatar_url", "") or "",
            server_id=snowflake_to_int_or_none(data.get("server_id")),
            channel_id=snowflake_to_int_or_none(data.get("channel_id")),
            message_id=snowflake_to_int_or_none(data.get("message_id")),
            dm_channel_id=snowflake_to_int_or_none(data.get("dm_channel_id")),
            read=bool(data.get("read", False)),
            created_at=data.get("created_at", "") or "",
            state=state,
        )

    async def mark_read(self) -> None:
        """Mark this notification as read."""
        await self._state.http.mark_notification_read(self.id)
        self.read = True

    def __repr__(self) -> str:
        return (
            f"<Notification id={self.id} type={self.type!r} "
            f"read={self.read} title={self.title!r}>"
        )

    def __eq__(self, other: object) -> bool:
        return isinstance(other, Notification) and self.id == other.id

    def __hash__(self) -> int:
        return hash(self.id)
