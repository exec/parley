"""
parley.models.theme
===================
User-customizable client theme models.
"""

from __future__ import annotations

from datetime import datetime
from typing import Any, Optional

from ..utils import snowflake_to_int, snowflake_to_int_or_none

__all__ = ["UserTheme", "ThemePreferences"]


def _parse_iso8601(value: Any) -> Optional[datetime]:
    if not value:
        return None
    if isinstance(value, datetime):
        return value
    s = str(value)
    if s.endswith("Z"):
        s = s[:-1] + "+00:00"
    try:
        return datetime.fromisoformat(s)
    except ValueError:
        return None


class UserTheme:
    """
    A user-authored theme — a CSS overlay anchored on top of a built-in base.

    Attributes
    ----------
    id:
        Snowflake ID of the theme row.
    name:
        Theme display name.
    css:
        Raw CSS body (subject to server-side validation).
    base_theme:
        ID of the built-in base (``rory``, ``neon-nights``, …).
    background_url:
        Optional background-image URL.
    share_token:
        Public share token, when this theme has been shared.
    source_share_token:
        Share token of the upstream theme this was installed from, if any.
    is_published:
        Whether the theme is publicly listed in the theme repo.
    is_featured:
        Whether the theme has been featured by a Parley admin.
    author_username:
        Author username (only populated for repo / public lookups).
    author_display_name:
        Author display name (only populated for repo / public lookups).
    created_at:
        Creation timestamp (UTC).
    """

    __slots__ = (
        "id",
        "name",
        "css",
        "base_theme",
        "background_url",
        "share_token",
        "source_share_token",
        "is_published",
        "is_featured",
        "author_username",
        "author_display_name",
        "created_at",
        "_state",
    )

    def __init__(
        self,
        *,
        id: int,
        name: str,
        css: str,
        base_theme: str,
        background_url: Optional[str] = None,
        share_token: Optional[str] = None,
        source_share_token: Optional[str] = None,
        is_published: bool = False,
        is_featured: bool = False,
        author_username: str = "",
        author_display_name: str = "",
        created_at: Optional[datetime] = None,
        state: Optional[Any] = None,
    ) -> None:
        self.id = id
        self.name = name
        self.css = css
        self.base_theme = base_theme
        self.background_url = background_url
        self.share_token = share_token
        self.source_share_token = source_share_token
        self.is_published = is_published
        self.is_featured = is_featured
        self.author_username = author_username
        self.author_display_name = author_display_name
        self.created_at = created_at
        self._state: Optional[Any] = state

    @classmethod
    def _from_data(cls, data: dict, state: Optional[Any] = None) -> "UserTheme":
        return cls(
            id=snowflake_to_int(data.get("id", 0) or 0),
            name=data.get("name", "") or "",
            css=data.get("css", "") or "",
            base_theme=data.get("base_theme", "") or "",
            background_url=data.get("background_url"),
            share_token=data.get("share_token"),
            source_share_token=data.get("source_share_token"),
            is_published=bool(data.get("is_published", False)),
            is_featured=bool(data.get("is_featured", False)),
            author_username=data.get("author_username", "") or "",
            author_display_name=data.get("author_display_name", "") or "",
            created_at=_parse_iso8601(data.get("created_at")),
            state=state,
        )

    def __repr__(self) -> str:
        return f"<UserTheme id={self.id} name={self.name!r} base_theme={self.base_theme!r}>"

    def __eq__(self, other: object) -> bool:
        return isinstance(other, UserTheme) and self.id == other.id

    def __hash__(self) -> int:
        return hash(self.id)


class ThemePreferences:
    """
    The current user's theme preferences and saved custom themes.

    Attributes
    ----------
    active_theme:
        ID of the active theme (a built-in name like ``rory``, or ``custom``).
    active_custom_theme_id:
        When ``active_theme == "custom"``, the ID of the active custom theme.
    custom_themes:
        All custom themes the current user has authored.
    """

    __slots__ = ("active_theme", "active_custom_theme_id", "custom_themes")

    def __init__(
        self,
        *,
        active_theme: str,
        active_custom_theme_id: Optional[int],
        custom_themes: list[UserTheme],
    ) -> None:
        self.active_theme = active_theme
        self.active_custom_theme_id = active_custom_theme_id
        self.custom_themes = custom_themes

    @classmethod
    def _from_data(cls, data: dict, state: Optional[Any] = None) -> "ThemePreferences":
        themes_raw = data.get("custom_themes") or []
        return cls(
            active_theme=data.get("active_theme", "rory") or "rory",
            active_custom_theme_id=snowflake_to_int_or_none(
                data.get("active_custom_theme_id")
            ),
            custom_themes=[UserTheme._from_data(t, state) for t in themes_raw],
        )

    def __repr__(self) -> str:
        return (
            f"<ThemePreferences active_theme={self.active_theme!r} "
            f"custom_themes={len(self.custom_themes)}>"
        )
