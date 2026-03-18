"""
parley.models.member
====================
:class:`Member` — a user's membership record within a specific server.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any, Optional

from ..utils import snowflake_to_int, snowflake_to_int_or_none

if TYPE_CHECKING:
    from ..state import ConnectionState

__all__ = ["Member"]


class Member:
    """
    Represents a user's membership in a Parley server.

    Attributes
    ----------
    id:
        Membership record ID (may differ from *user_id*).
    server_id:
        ID of the server this membership belongs to.
    user_id:
        The :attr:`~parley.models.user.User.id` of the user.
    username:
        User's username.
    display_name:
        User's display name; falls back to *username*.
    avatar_url:
        User's avatar URL.
    is_bot:
        Whether this member is a bot account.
    bot_degraded:
        Whether the bot is currently in a degraded/error state.
    roles:
        List of role IDs assigned to this member.
    """

    __slots__ = (
        "id",
        "server_id",
        "user_id",
        "username",
        "display_name",
        "avatar_url",
        "is_bot",
        "bot_degraded",
        "roles",
        "_state",
    )

    def __init__(
        self,
        *,
        id: int,
        server_id: int,
        user_id: int,
        username: str,
        display_name: str,
        avatar_url: str,
        is_bot: bool,
        bot_degraded: bool,
        roles: list[int],
        state: Optional[Any] = None,
    ) -> None:
        self.id = id
        self.server_id = server_id
        self.user_id = user_id
        self.username = username
        self.display_name = display_name
        self.avatar_url = avatar_url
        self.is_bot = is_bot
        self.bot_degraded = bot_degraded
        self.roles: list[int] = roles
        self._state: Optional[Any] = state

    @classmethod
    def _from_data(cls, data: dict, state: Optional[Any] = None) -> "Member":
        raw_roles = data.get("roles") or []
        roles = [int(r) for r in raw_roles if r is not None]
        return cls(
            id=snowflake_to_int(data.get("id") or data.get("user_id", 0)),
            server_id=snowflake_to_int(data.get("server_id", 0) or 0),
            user_id=snowflake_to_int(data.get("user_id", 0) or 0),
            username=data.get("username", ""),
            display_name=data.get("display_name") or data.get("username", ""),
            avatar_url=data.get("avatar_url", "") or "",
            is_bot=bool(data.get("is_bot", False)),
            bot_degraded=bool(data.get("bot_degraded", False)),
            roles=roles,
            state=state,
        )

    def _update(self, data: dict) -> None:
        """Apply partial updates (e.g. from ``MEMBER_ROLE_UPDATE`` or ``SERVER_MEMBER_JOIN``)."""
        if "username" in data:
            self.username = data["username"]
        if "display_name" in data:
            self.display_name = data["display_name"] or self.username
        if "avatar_url" in data:
            self.avatar_url = data["avatar_url"] or ""
        if "is_bot" in data:
            self.is_bot = bool(data["is_bot"])
        if "bot_degraded" in data:
            self.bot_degraded = bool(data["bot_degraded"])
        if "roles" in data and data["roles"] is not None:
            self.roles = [int(r) for r in data["roles"]]

    @property
    def display(self) -> str:
        """Returns *display_name* if set, otherwise *username*."""
        return self.display_name or self.username

    @property
    def mention(self) -> str:
        """Mention string ``<@user_id>``."""
        return f"<@{self.user_id}>"

    async def kick(self) -> None:
        """Kick this member from the server."""
        await self._state.http.kick_member(self.server_id, self.user_id)

    async def ban(self) -> None:
        """Ban this member from the server."""
        await self._state.http.ban_member(self.server_id, self.user_id)

    async def add_role(self, role_id: int) -> None:
        """Add a role to this member."""
        await self._state.http.add_member_role(self.server_id, self.user_id, role_id)

    async def remove_role(self, role_id: int) -> None:
        """Remove a role from this member."""
        await self._state.http.remove_member_role(
            self.server_id, self.user_id, role_id
        )

    def __repr__(self) -> str:
        return (
            f"<Member user_id={self.user_id} username={self.username!r} "
            f"server_id={self.server_id}>"
        )

    def __eq__(self, other: object) -> bool:
        return (
            isinstance(other, Member)
            and self.user_id == other.user_id
            and self.server_id == other.server_id
        )

    def __hash__(self) -> int:
        return hash((self.user_id, self.server_id))
