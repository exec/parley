"""
parley.models.server
====================
:class:`Server` model representing a Parley server (guild).
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any, Optional

from ..utils import snowflake_to_int

if TYPE_CHECKING:
    from ..state import ConnectionState

__all__ = ["Server"]


class Server:
    """
    A Parley server (analogous to a Discord guild).

    Attributes
    ----------
    id:
        Integer snowflake ID.
    name:
        Human-readable server name.
    icon_url:
        URL of the server icon (may be empty string).
    owner_id:
        The :attr:`~parley.models.user.User.id` of the server owner.
    """

    __slots__ = ("id", "name", "icon_url", "owner_id", "_state")

    def __init__(
        self,
        *,
        id: int,
        name: str,
        icon_url: str,
        owner_id: int,
        state: Optional[Any] = None,
    ) -> None:
        self.id = id
        self.name = name
        self.icon_url = icon_url
        self.owner_id = owner_id
        self._state: Optional[Any] = state

    @classmethod
    def _from_data(cls, data: dict, state: Optional[Any] = None) -> "Server":
        return cls(
            id=snowflake_to_int(data["id"]),
            name=data.get("name", ""),
            icon_url=data.get("icon_url", "") or "",
            owner_id=snowflake_to_int(data.get("owner_id", 0) or 0),
            state=state,
        )

    def _update(self, data: dict) -> None:
        """Apply a partial update dict (e.g. from ``SERVER_UPDATE`` WS event)."""
        if "name" in data:
            self.name = data["name"]
        if "icon_url" in data:
            self.icon_url = data["icon_url"] or ""

    # ------------------------------------------------------------------
    # Async helpers  (require _state to be set)
    # ------------------------------------------------------------------

    async def edit(
        self,
        *,
        name: Optional[str] = None,
        icon_url: Optional[str] = None,
    ) -> "Server":
        """Edit this server's settings.

        Parameters
        ----------
        name:
            New server name.
        icon_url:
            New icon URL.

        Returns
        -------
        :class:`Server`
            The updated server object.
        """
        data = await self._state.http.edit_server(
            self.id, name=name, icon_url=icon_url
        )
        self._update(data)
        return self

    async def delete(self) -> None:
        """Delete this server (owner only)."""
        await self._state.http.delete_server(self.id)

    async def leave(self) -> None:
        """Leave this server (non-owner members)."""
        await self._state.http.leave_server(self.id)

    async def fetch_members(self) -> list:
        """Fetch the current member list from the API.

        Returns
        -------
        list[:class:`~parley.models.member.Member`]
        """
        from .member import Member

        data = await self._state.http.get_server_members(self.id)
        return [Member._from_data(m, self._state) for m in data]

    async def fetch_channels(self) -> list:
        """Fetch the channel list from the API.

        Returns
        -------
        list[:class:`~parley.models.channel.Channel`]
        """
        from .channel import channel_from_data

        data = await self._state.http.get_server_channels(self.id)
        return [channel_from_data(c, self._state) for c in data]

    async def create_channel(
        self,
        name: str,
        *,
        channel_type: int = 0,
        topic: Optional[str] = None,
        parent_id: Optional[int] = None,
    ):
        """Create a new channel in this server.

        Returns
        -------
        :class:`~parley.models.channel.Channel`
        """
        from .channel import channel_from_data

        data = await self._state.http.create_channel(
            self.id,
            name=name,
            channel_type=channel_type,
            topic=topic,
            parent_id=parent_id,
        )
        return channel_from_data(data, self._state)

    async def create_invite(self) -> str:
        """Create an invite and return its code."""
        data = await self._state.http.create_invite(self.id)
        return data.get("code", "")

    async def fetch_roles(self) -> list:
        """Fetch the roles list from the API.

        Returns
        -------
        list[:class:`~parley.models.role.Role`]
        """
        from .role import Role

        data = await self._state.http.get_roles(self.id)
        return [Role._from_data(r, self._state) for r in data]

    async def kick(self, user_id: int) -> None:
        """Kick a member by user ID."""
        await self._state.http.kick_member(self.id, user_id)

    async def ban(self, user_id: int) -> None:
        """Ban a member by user ID."""
        await self._state.http.ban_member(self.id, user_id)

    def __repr__(self) -> str:
        return f"<Server id={self.id} name={self.name!r}>"

    def __eq__(self, other: object) -> bool:
        return isinstance(other, Server) and self.id == other.id

    def __hash__(self) -> int:
        return hash(self.id)
