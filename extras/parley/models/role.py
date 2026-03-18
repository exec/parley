"""
parley.models.role
==================
:class:`Role` — a permission role within a Parley server.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any, Optional

from ..enums import Permissions
from ..utils import snowflake_to_int

if TYPE_CHECKING:
    from ..state import ConnectionState

__all__ = ["Role"]


class Role:
    """
    A role that can be assigned to server members.

    Attributes
    ----------
    id:
        Integer snowflake ID.
    server_id:
        ID of the server this role belongs to.
    name:
        Role name.
    color:
        Hex colour string (e.g. ``#ff0000``) or empty string.
    permissions:
        :class:`~parley.enums.Permissions` bit-flag value.
    hoist:
        Whether this role is shown separately in the member list.
    position:
        Ordering position (higher = more powerful).
    is_everyone:
        Whether this is the special ``@everyone`` role.
    """

    __slots__ = (
        "id",
        "server_id",
        "name",
        "color",
        "permissions",
        "hoist",
        "position",
        "is_everyone",
        "_state",
    )

    def __init__(
        self,
        *,
        id: int,
        server_id: int,
        name: str,
        color: str,
        permissions: int,
        hoist: bool,
        position: int,
        is_everyone: bool,
        state: Optional[Any] = None,
    ) -> None:
        self.id = id
        self.server_id = server_id
        self.name = name
        self.color = color
        self.permissions = Permissions(permissions)
        self.hoist = hoist
        self.position = position
        self.is_everyone = is_everyone
        self._state: Optional[Any] = state

    @classmethod
    def _from_data(cls, data: dict, state: Optional[Any] = None) -> "Role":
        return cls(
            id=snowflake_to_int(data["id"]),
            server_id=snowflake_to_int(data.get("server_id", 0) or 0),
            name=data.get("name", ""),
            color=data.get("color", "") or "",
            permissions=int(data.get("permissions", 0) or 0),
            hoist=bool(data.get("hoist", False)),
            position=int(data.get("position", 0) or 0),
            is_everyone=bool(data.get("is_everyone", False)),
            state=state,
        )

    def _update(self, data: dict) -> None:
        """Apply a partial update from ``ROLE_UPDATE`` WS event."""
        if "name" in data:
            self.name = data["name"]
        if "color" in data:
            self.color = data["color"] or ""
        if "permissions" in data:
            self.permissions = Permissions(int(data["permissions"] or 0))
        if "hoist" in data:
            self.hoist = bool(data["hoist"])
        if "position" in data:
            self.position = int(data["position"])

    async def edit(
        self,
        *,
        name: Optional[str] = None,
        color: Optional[str] = None,
        permissions: Optional[int] = None,
        hoist: Optional[bool] = None,
    ) -> "Role":
        """Edit this role.

        Returns
        -------
        :class:`Role`
            The updated role object.
        """
        fields: dict = {}
        if name is not None:
            fields["name"] = name
        if color is not None:
            fields["color"] = color
        if permissions is not None:
            fields["permissions"] = permissions
        if hoist is not None:
            fields["hoist"] = hoist
        data = await self._state.http.edit_role(self.server_id, self.id, **fields)
        self._update(data)
        return self

    async def delete(self) -> None:
        """Delete this role."""
        await self._state.http.delete_role(self.server_id, self.id)

    def __repr__(self) -> str:
        return f"<Role id={self.id} name={self.name!r} server_id={self.server_id}>"

    def __eq__(self, other: object) -> bool:
        return isinstance(other, Role) and self.id == other.id

    def __hash__(self) -> int:
        return hash(self.id)
