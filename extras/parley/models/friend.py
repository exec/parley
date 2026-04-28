"""
parley.models.friend
====================
Friend system model classes.

- :class:`Friend` — an accepted friendship row (the other party's profile).
- :class:`FriendRequest` — a pending friend request, incoming or outgoing.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any, Optional

from ..utils import snowflake_to_int, snowflake_to_int_or_none

if TYPE_CHECKING:
    from ..state import ConnectionState

__all__ = ["Friend", "FriendRequest"]


class Friend:
    """
    An accepted friendship — represents the *other* party.

    Attributes
    ----------
    id:
        Integer snowflake ID of the other user.
    username:
        Username of the other user.
    display_name:
        Display name; falls back to *username* when empty.
    avatar_url:
        Avatar URL (may be empty string).
    """

    __slots__ = (
        "id",
        "username",
        "display_name",
        "avatar_url",
        "_state",
    )

    def __init__(
        self,
        *,
        id: int,
        username: str,
        display_name: str,
        avatar_url: str,
        state: Optional[Any] = None,
    ) -> None:
        self.id = id
        self.username = username
        self.display_name = display_name
        self.avatar_url = avatar_url
        self._state: Optional[Any] = state

    @classmethod
    def _from_data(cls, data: dict, state: Optional[Any] = None) -> "Friend":
        return cls(
            id=snowflake_to_int(data["id"]),
            username=data.get("username", "") or "",
            display_name=data.get("display_name") or data.get("username", "") or "",
            avatar_url=data.get("avatar_url", "") or "",
            state=state,
        )

    @property
    def display(self) -> str:
        """Returns *display_name* if set, otherwise *username*."""
        return self.display_name or self.username

    @property
    def mention(self) -> str:
        """Mention string ``<@id>``."""
        return f"<@{self.id}>"

    async def remove(self) -> None:
        """Unfriend this user (``DELETE /api/friends/{id}``)."""
        await self._state.http.remove_friend(self.id)

    async def block(self) -> None:
        """Block this user (``POST /api/users/{id}/block``)."""
        await self._state.http.block_user(self.id)

    def __repr__(self) -> str:
        return f"<Friend id={self.id} username={self.username!r}>"

    def __eq__(self, other: object) -> bool:
        return isinstance(other, Friend) and self.id == other.id

    def __hash__(self) -> int:
        return hash(self.id)


class FriendRequest:
    """
    A pending friend request — either incoming or outgoing.

    Attributes
    ----------
    id:
        Integer snowflake ID of the friend-request row.
    sender_id:
        User ID of the user who sent the request.
    receiver_id:
        User ID of the recipient.
    status:
        Server-side status string (typically ``"pending"``).
    user_id:
        ID of the *other* party (the one this request is paired with).
    username:
        Username of the other party.
    display_name:
        Display name of the other party.
    avatar_url:
        Avatar URL of the other party.
    created_at:
        ISO-8601 creation timestamp.
    """

    __slots__ = (
        "id",
        "sender_id",
        "receiver_id",
        "status",
        "user_id",
        "username",
        "display_name",
        "avatar_url",
        "created_at",
        "_state",
    )

    def __init__(
        self,
        *,
        id: int,
        sender_id: int,
        receiver_id: int,
        status: str,
        user_id: int,
        username: str,
        display_name: str,
        avatar_url: str,
        created_at: str,
        state: Optional[Any] = None,
    ) -> None:
        self.id = id
        self.sender_id = sender_id
        self.receiver_id = receiver_id
        self.status = status
        self.user_id = user_id
        self.username = username
        self.display_name = display_name
        self.avatar_url = avatar_url
        self.created_at = created_at
        self._state: Optional[Any] = state

    @classmethod
    def _from_data(cls, data: dict, state: Optional[Any] = None) -> "FriendRequest":
        user = data.get("user") or {}
        return cls(
            id=snowflake_to_int(data["id"]),
            sender_id=snowflake_to_int(data.get("sender_id", 0) or 0),
            receiver_id=snowflake_to_int(data.get("receiver_id", 0) or 0),
            status=data.get("status", "") or "",
            user_id=snowflake_to_int_or_none(user.get("id")) or 0,
            username=user.get("username", "") or "",
            display_name=user.get("display_name") or user.get("username", "") or "",
            avatar_url=user.get("avatar_url", "") or "",
            created_at=data.get("created_at", "") or "",
            state=state,
        )

    async def accept(self) -> "Friend":
        """Accept this request — only valid for *incoming* requests.

        Returns
        -------
        :class:`Friend`
            The newly created friendship row.
        """
        data = await self._state.http.accept_friend_request(self.id)
        return Friend._from_data(data, self._state)

    async def decline(self) -> None:
        """Decline (incoming) or cancel (outgoing) this request."""
        await self._state.http.decline_friend_request(self.id)

    # Alias — same endpoint covers both incoming-decline and outgoing-cancel.
    async def cancel(self) -> None:
        """Cancel an outgoing request (alias for :meth:`decline`)."""
        await self._state.http.decline_friend_request(self.id)

    def __repr__(self) -> str:
        return (
            f"<FriendRequest id={self.id} status={self.status!r} "
            f"username={self.username!r}>"
        )

    def __eq__(self, other: object) -> bool:
        return isinstance(other, FriendRequest) and self.id == other.id

    def __hash__(self) -> int:
        return hash(self.id)
