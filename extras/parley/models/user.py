"""
parley.models.user
==================
User model classes.

- :class:`User` — the authenticated client's own profile (includes email).
- :class:`ClientUser` — alias for :class:`User` for discord.py naming familiarity.
- :class:`PublicUser` — another user's public-facing profile.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any, Optional

from ..enums import Badges
from ..utils import snowflake_to_int, snowflake_to_int_or_none

if TYPE_CHECKING:
    from ..state import ConnectionState

__all__ = ["User", "ClientUser", "PublicUser"]


class PublicUser:
    """
    Public profile of a Parley user visible to anyone.

    Attributes
    ----------
    id:
        Integer snowflake ID.
    username:
        Unique username (e.g. ``john_doe``).
    display_name:
        Optional display name; falls back to *username* when not set.
    avatar_url:
        URL of the user's avatar image (may be empty string).
    banner_url:
        URL of the user's profile banner (may be empty string).
    bio:
        Short biography text.
    badges:
        :class:`~parley.enums.Badges` bit-flag value.
    """

    __slots__ = (
        "id",
        "username",
        "display_name",
        "avatar_url",
        "banner_url",
        "bio",
        "badges",
        "_state",
    )

    def __init__(
        self,
        *,
        id: int,
        username: str,
        display_name: str,
        avatar_url: str,
        banner_url: str,
        bio: str,
        badges: int,
        state: Optional[Any] = None,
    ) -> None:
        self.id = id
        self.username = username
        self.display_name = display_name
        self.avatar_url = avatar_url
        self.banner_url = banner_url
        self.bio = bio
        self.badges = Badges(badges)
        self._state = state

    @classmethod
    def _from_data(cls, data: dict, state: Optional[Any] = None) -> "PublicUser":
        return cls(
            id=snowflake_to_int(data["id"]),
            username=data.get("username", ""),
            display_name=data.get("display_name") or data.get("username", ""),
            avatar_url=data.get("avatar_url", "") or "",
            banner_url=data.get("banner_url", "") or "",
            bio=data.get("bio", "") or "",
            badges=data.get("badges", 0) or 0,
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

    def __repr__(self) -> str:
        return f"<PublicUser id={self.id} username={self.username!r}>"

    def __eq__(self, other: object) -> bool:
        return isinstance(other, PublicUser) and self.id == other.id

    def __hash__(self) -> int:
        return hash(self.id)


class User(PublicUser):
    """
    The authenticated client's own profile.

    Includes private fields not available on :class:`PublicUser`.

    Attributes
    ----------
    email:
        The user's email address.
    email_verified:
        Whether the email address has been verified.
    """

    __slots__ = PublicUser.__slots__ + ("email", "email_verified")  # type: ignore[assignment]

    def __init__(
        self,
        *,
        id: int,
        username: str,
        display_name: str,
        avatar_url: str,
        banner_url: str,
        bio: str,
        badges: int,
        email: str,
        email_verified: bool,
        state: Optional[Any] = None,
    ) -> None:
        super().__init__(
            id=id,
            username=username,
            display_name=display_name,
            avatar_url=avatar_url,
            banner_url=banner_url,
            bio=bio,
            badges=badges,
            state=state,
        )
        self.email = email
        self.email_verified = email_verified

    @classmethod
    def _from_data(cls, data: dict, state: Optional[Any] = None) -> "User":  # type: ignore[override]
        return cls(
            id=snowflake_to_int(data["id"]),
            username=data.get("username", ""),
            display_name=data.get("display_name") or data.get("username", ""),
            avatar_url=data.get("avatar_url", "") or "",
            banner_url=data.get("banner_url", "") or "",
            bio=data.get("bio", "") or "",
            badges=data.get("badges", 0) or 0,
            email=data.get("email", "") or "",
            email_verified=bool(data.get("email_verified", False)),
            state=state,
        )

    def __repr__(self) -> str:
        return f"<User id={self.id} username={self.username!r} email={self.email!r}>"


#: Alias for :class:`User` matching discord.py naming convention.
ClientUser = User
