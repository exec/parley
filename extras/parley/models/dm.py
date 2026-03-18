"""
parley.models.dm
================
:class:`DmChannel` — a direct message channel between two users.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any, Optional

from ..utils import snowflake_to_int

if TYPE_CHECKING:
    from ..state import ConnectionState

__all__ = ["DmChannel"]


class DmChannel:
    """
    A direct message (DM) channel between the authenticated user and one other user.

    Attributes
    ----------
    id:
        Integer snowflake ID of the DM channel.
    other_user_id:
        User ID of the other participant.
    other_username:
        Username of the other participant.
    other_avatar_url:
        Avatar URL of the other participant.
    """

    __slots__ = (
        "id",
        "other_user_id",
        "other_username",
        "other_avatar_url",
        "_state",
    )

    def __init__(
        self,
        *,
        id: int,
        other_user_id: int,
        other_username: str,
        other_avatar_url: str,
        state: Optional[Any] = None,
    ) -> None:
        self.id = id
        self.other_user_id = other_user_id
        self.other_username = other_username
        self.other_avatar_url = other_avatar_url
        self._state: Optional[Any] = state

    @classmethod
    def _from_data(cls, data: dict, state: Optional[Any] = None) -> "DmChannel":
        return cls(
            id=snowflake_to_int(data["id"]),
            other_user_id=snowflake_to_int(data.get("other_user_id", 0) or 0),
            other_username=data.get("other_username", ""),
            other_avatar_url=data.get("other_avatar_url", "") or "",
            state=state,
        )

    # ------------------------------------------------------------------
    # Async helpers
    # ------------------------------------------------------------------

    async def send(self, content: str):
        """Send a message in this DM channel.

        Parameters
        ----------
        content:
            Text content to send.

        Returns
        -------
        :class:`~parley.models.message.DmMessage`
        """
        from .message import DmMessage

        data = await self._state.http.create_dm_message(self.id, content)
        return DmMessage._from_data(data, self._state)

    async def fetch_messages(
        self,
        *,
        limit: int = 50,
        before: Optional[int] = None,
    ) -> list:
        """Fetch DM message history.

        Parameters
        ----------
        limit:
            Number of messages to return (max 50).
        before:
            Return messages before this message ID.

        Returns
        -------
        list[:class:`~parley.models.message.DmMessage`]
        """
        from .message import DmMessage

        data = await self._state.http.get_dm_messages(
            self.id, limit=limit, before=before
        )
        return [DmMessage._from_data(m, self._state) for m in data]

    def __repr__(self) -> str:
        return (
            f"<DmChannel id={self.id} other_user_id={self.other_user_id} "
            f"other_username={self.other_username!r}>"
        )

    def __eq__(self, other: object) -> bool:
        return isinstance(other, DmChannel) and self.id == other.id

    def __hash__(self) -> int:
        return hash(self.id)
