"""
parley.models.message
=====================
Message model classes.

- :class:`Message`    — a message posted in a server text channel.
- :class:`DmMessage`  — a message posted in a DM conversation.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any, Optional

from ..utils import parse_mentions, snowflake_to_int, snowflake_to_int_or_none

if TYPE_CHECKING:
    from ..state import ConnectionState

__all__ = ["Message", "DmMessage"]


class Message:
    """
    A message sent in a server text channel.

    Attributes
    ----------
    id:
        Integer snowflake ID.
    channel_id:
        ID of the channel this message belongs to.
    author_id:
        User ID of the message author.
    author_username:
        Username of the author.
    author_display_name:
        Display name of the author; falls back to *author_username*.
    author_avatar_url:
        Avatar URL of the author.
    author_is_bot:
        Whether the author is a bot.
    content:
        Text content of the message.
    attachment_url:
        URL of a file attachment (empty string if none).
    attachment_name:
        Filename of the attachment (empty string if none).
    parent_id:
        ID of the parent message if this is a thread reply, else ``None``.
    via_api:
        Whether this message was sent via the API (bot/key).
    created_at:
        ISO-8601 creation timestamp string.
    updated_at:
        ISO-8601 last-edit timestamp string.
    """

    __slots__ = (
        "id",
        "channel_id",
        "author_id",
        "author_username",
        "author_display_name",
        "author_avatar_url",
        "author_is_bot",
        "content",
        "attachment_url",
        "attachment_name",
        "parent_id",
        "via_api",
        "created_at",
        "updated_at",
        "_state",
    )

    def __init__(
        self,
        *,
        id: int,
        channel_id: int,
        author_id: int,
        author_username: str,
        author_display_name: str,
        author_avatar_url: str,
        author_is_bot: bool,
        content: str,
        attachment_url: str,
        attachment_name: str,
        parent_id: Optional[int],
        via_api: bool,
        created_at: str,
        updated_at: str,
        state: Optional[Any] = None,
    ) -> None:
        self.id = id
        self.channel_id = channel_id
        self.author_id = author_id
        self.author_username = author_username
        self.author_display_name = author_display_name
        self.author_avatar_url = author_avatar_url
        self.author_is_bot = author_is_bot
        self.content = content
        self.attachment_url = attachment_url
        self.attachment_name = attachment_name
        self.parent_id = parent_id
        self.via_api = via_api
        self.created_at = created_at
        self.updated_at = updated_at
        self._state: Optional[Any] = state

    @classmethod
    def _from_data(cls, data: dict, state: Optional[Any] = None) -> "Message":
        return cls(
            id=snowflake_to_int(data["id"]),
            channel_id=snowflake_to_int(data.get("channel_id", 0) or 0),
            author_id=snowflake_to_int(data.get("author_id", 0) or 0),
            author_username=data.get("author_username", ""),
            author_display_name=(
                data.get("author_display_name") or data.get("author_username", "")
            ),
            author_avatar_url=data.get("author_avatar_url", "") or "",
            author_is_bot=bool(data.get("author_is_bot", False)),
            content=data.get("content", ""),
            attachment_url=data.get("attachment_url", "") or "",
            attachment_name=data.get("attachment_name", "") or "",
            parent_id=snowflake_to_int_or_none(data.get("parent_id")),
            via_api=bool(data.get("via_api", False)),
            created_at=data.get("created_at", "") or "",
            updated_at=data.get("updated_at", "") or "",
            state=state,
        )

    def _update(self, data: dict) -> None:
        """Apply a partial update from a ``MESSAGE_UPDATE`` WS event."""
        if "content" in data:
            self.content = data["content"]
        if "updated_at" in data:
            self.updated_at = data["updated_at"] or ""
        if "attachment_url" in data:
            self.attachment_url = data["attachment_url"] or ""
        if "attachment_name" in data:
            self.attachment_name = data["attachment_name"] or ""

    # ------------------------------------------------------------------
    # Convenience properties
    # ------------------------------------------------------------------

    @property
    def author_display(self) -> str:
        """Returns *author_display_name* if set, otherwise *author_username*."""
        return self.author_display_name or self.author_username

    # Back-compat alias used by the old selfbot code.
    @property
    def author_name(self) -> str:
        """Alias for :attr:`author_display`."""
        return self.author_display

    def mentions(self) -> list[int]:
        """Return list of user IDs mentioned in this message's content."""
        return parse_mentions(self.content)

    def mentions_user(self, user_id: int | str) -> bool:
        """Return ``True`` if *user_id* is mentioned in this message."""
        return int(user_id) in self.mentions()

    # ------------------------------------------------------------------
    # Async helpers (require _state)
    # ------------------------------------------------------------------

    async def reply(self, content: str) -> "Message":
        """Send a thread reply to this message.

        Parameters
        ----------
        content:
            Text of the reply.

        Returns
        -------
        :class:`Message`
            The newly created reply message.
        """
        data = await self._state.http.create_message(
            self.channel_id, content, parent_id=self.id
        )
        return Message._from_data(data, self._state)

    async def edit(self, content: str) -> "Message":
        """Edit this message.

        Parameters
        ----------
        content:
            New text content.

        Returns
        -------
        :class:`Message`
            The updated message.
        """
        data = await self._state.http.edit_message(self.id, content)
        self._update(data)
        return self

    async def delete(self) -> None:
        """Delete this message."""
        await self._state.http.delete_message(self.id)

    async def add_reaction(self, emoji: str) -> None:
        """Toggle a reaction on this message.

        Parameters
        ----------
        emoji:
            Emoji string (e.g. ``"👍"``).
        """
        await self._state.http.add_reaction(self.id, emoji)

    def __repr__(self) -> str:
        snippet = self.content[:40].replace("\n", " ")
        return (
            f"<Message id={self.id} author_id={self.author_id} "
            f"channel_id={self.channel_id} content={snippet!r}>"
        )

    def __eq__(self, other: object) -> bool:
        return isinstance(other, Message) and self.id == other.id

    def __hash__(self) -> int:
        return hash(self.id)


class DmMessage:
    """
    A message sent in a DM (direct message) conversation.

    Attributes
    ----------
    id:
        Integer snowflake ID.
    dm_channel_id:
        ID of the :class:`~parley.models.dm.DmChannel`.
    author_id:
        User ID of the message author.
    author_username:
        Username of the author.
    author_display_name:
        Display name of the author.
    content:
        Text content.
    created_at:
        ISO-8601 creation timestamp string.
    """

    __slots__ = (
        "id",
        "dm_channel_id",
        "author_id",
        "author_username",
        "author_display_name",
        "content",
        "created_at",
        "_state",
    )

    def __init__(
        self,
        *,
        id: int,
        dm_channel_id: int,
        author_id: int,
        author_username: str,
        author_display_name: str,
        content: str,
        created_at: str,
        state: Optional[Any] = None,
    ) -> None:
        self.id = id
        self.dm_channel_id = dm_channel_id
        self.author_id = author_id
        self.author_username = author_username
        self.author_display_name = author_display_name
        self.content = content
        self.created_at = created_at
        self._state: Optional[Any] = state

    @classmethod
    def _from_data(cls, data: dict, state: Optional[Any] = None) -> "DmMessage":
        return cls(
            id=snowflake_to_int(data["id"]),
            dm_channel_id=snowflake_to_int(
                data.get("dm_channel_id") or data.get("channel_id", 0) or 0
            ),
            author_id=snowflake_to_int(data.get("author_id", 0) or 0),
            author_username=data.get("author_username", ""),
            author_display_name=(
                data.get("author_display_name") or data.get("author_username", "")
            ),
            content=data.get("content", ""),
            created_at=data.get("created_at", "") or "",
            state=state,
        )

    @property
    def author_display(self) -> str:
        """Returns *author_display_name* if set, otherwise *author_username*."""
        return self.author_display_name or self.author_username

    async def reply(self, content: str) -> "DmMessage":
        """Reply in the same DM channel.

        Parameters
        ----------
        content:
            Text of the reply.

        Returns
        -------
        :class:`DmMessage`
        """
        data = await self._state.http.create_dm_message(self.dm_channel_id, content)
        return DmMessage._from_data(data, self._state)

    def __repr__(self) -> str:
        snippet = self.content[:40].replace("\n", " ")
        return (
            f"<DmMessage id={self.id} author_id={self.author_id} "
            f"dm_channel_id={self.dm_channel_id} content={snippet!r}>"
        )

    def __eq__(self, other: object) -> bool:
        return isinstance(other, DmMessage) and self.id == other.id

    def __hash__(self) -> int:
        return hash(self.id)
