"""
parley.models.invite
====================
:class:`Invite` — a server invite object.
"""

from __future__ import annotations

from typing import Any, Optional

from ..utils import snowflake_to_int

__all__ = ["Invite"]


class Invite:
    """
    A Parley server invite.

    Invites can be fetched without authentication via
    :meth:`~parley.Client.fetch_invite`.

    Attributes
    ----------
    code:
        The unique invite code string (used in invite URLs).
    server_id:
        ID of the server this invite grants access to.
    server_name:
        Name of the target server.
    inviter_id:
        User ID of the member who created the invite.
    inviter_username:
        Username of the inviter.
    """

    __slots__ = (
        "code",
        "server_id",
        "server_name",
        "inviter_id",
        "inviter_username",
        "_state",
    )

    def __init__(
        self,
        *,
        code: str,
        server_id: int,
        server_name: str,
        inviter_id: int,
        inviter_username: str,
        state: Optional[Any] = None,
    ) -> None:
        self.code = code
        self.server_id = server_id
        self.server_name = server_name
        self.inviter_id = inviter_id
        self.inviter_username = inviter_username
        self._state: Optional[Any] = state

    @classmethod
    def _from_data(cls, data: dict, state: Optional[Any] = None) -> "Invite":
        return cls(
            code=data.get("code", ""),
            server_id=snowflake_to_int(data.get("server_id", 0) or 0),
            server_name=data.get("server_name", ""),
            inviter_id=snowflake_to_int(data.get("inviter_id", 0) or 0),
            inviter_username=data.get("inviter_username", ""),
            state=state,
        )

    @property
    def url(self) -> str:
        """Full invite URL (uses the base URL from state if available)."""
        if self._state and hasattr(self._state, "http"):
            base = self._state.http.base_url.rstrip("/")
            return f"{base}/invite/{self.code}"
        return self.code

    def __repr__(self) -> str:
        return (
            f"<Invite code={self.code!r} server_name={self.server_name!r} "
            f"server_id={self.server_id}>"
        )

    def __str__(self) -> str:
        return self.code

    def __eq__(self, other: object) -> bool:
        return isinstance(other, Invite) and self.code == other.code

    def __hash__(self) -> int:
        return hash(self.code)
