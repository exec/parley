"""
parley.models.bin
=================
Models for the **bin** feature — a pastebin-style code-sharing channel
with versioning, line comments, and admin tags.
"""

from __future__ import annotations

from datetime import datetime
from typing import Any, Optional

from ..utils import snowflake_to_int, snowflake_to_int_or_none

__all__ = [
    "BinPostFile",
    "BinPost",
    "BinPostVersionFile",
    "BinPostVersion",
    "BinLineComment",
    "BinChannelTag",
]


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


class BinPostFile:
    """
    A single file attached to a bin post.

    Attributes
    ----------
    id:
        Snowflake ID of the file row (0 for unsaved client-built files).
    post_id:
        Owning post ID, or 0 when constructing for a create/edit request.
    filename:
        File name including extension.
    language:
        Language hint (e.g. ``python``, ``go``, ``ts``); empty if unknown.
    content:
        Raw file body.
    position:
        Display ordering (lower = first).
    """

    __slots__ = ("id", "post_id", "filename", "language", "content", "position")

    def __init__(
        self,
        *,
        filename: str,
        content: str,
        language: str = "",
        position: int = 0,
        id: int = 0,
        post_id: int = 0,
    ) -> None:
        self.id = id
        self.post_id = post_id
        self.filename = filename
        self.language = language
        self.content = content
        self.position = position

    @classmethod
    def _from_data(cls, data: dict) -> "BinPostFile":
        return cls(
            id=snowflake_to_int(data.get("id", 0) or 0),
            post_id=snowflake_to_int(data.get("post_id", 0) or 0),
            filename=data.get("filename", "") or "",
            language=data.get("language", "") or "",
            content=data.get("content", "") or "",
            position=int(data.get("position", 0) or 0),
        )

    def to_dict(self) -> dict:
        """Serialise for inclusion in a create/edit-post request body."""
        return {
            "filename": self.filename,
            "language": self.language,
            "content": self.content,
            "position": self.position,
        }

    def __repr__(self) -> str:
        return f"<BinPostFile filename={self.filename!r} language={self.language!r}>"


class BinPost:
    """
    A bin post — a titled, tagged collection of code files within a bin
    channel.

    Attributes
    ----------
    id:
        Snowflake ID of the post.
    channel_id:
        Bin channel the post lives in.
    thread_channel_id:
        Auto-created discussion thread channel for top-level comments.
    author_id:
        Author user ID.
    title:
        Post title.
    description:
        Optional long-form description (Markdown).
    tags:
        List of tag names attached to the post.
    files:
        List of :class:`BinPostFile` rows.
    author_username:
        Author username (server-computed).
    author_avatar_url:
        Author avatar URL (server-computed).
    comment_count:
        Top-level comment count in the discussion thread.
    line_comment_count:
        Inline (per-line) comment count across all versions.
    version_count:
        Number of saved versions for this post.
    created_at:
        Creation timestamp.
    updated_at:
        Last edit timestamp.
    """

    __slots__ = (
        "id",
        "channel_id",
        "thread_channel_id",
        "author_id",
        "title",
        "description",
        "tags",
        "files",
        "author_username",
        "author_avatar_url",
        "comment_count",
        "line_comment_count",
        "version_count",
        "created_at",
        "updated_at",
        "_state",
    )

    def __init__(
        self,
        *,
        id: int,
        channel_id: int,
        thread_channel_id: int,
        author_id: int,
        title: str,
        description: str,
        tags: list[str],
        files: list[BinPostFile],
        author_username: str = "",
        author_avatar_url: str = "",
        comment_count: int = 0,
        line_comment_count: int = 0,
        version_count: int = 0,
        created_at: Optional[datetime] = None,
        updated_at: Optional[datetime] = None,
        state: Optional[Any] = None,
    ) -> None:
        self.id = id
        self.channel_id = channel_id
        self.thread_channel_id = thread_channel_id
        self.author_id = author_id
        self.title = title
        self.description = description
        self.tags = tags
        self.files = files
        self.author_username = author_username
        self.author_avatar_url = author_avatar_url
        self.comment_count = comment_count
        self.line_comment_count = line_comment_count
        self.version_count = version_count
        self.created_at = created_at
        self.updated_at = updated_at
        self._state: Optional[Any] = state

    @classmethod
    def _from_data(cls, data: dict, state: Optional[Any] = None) -> "BinPost":
        files_raw = data.get("files") or []
        tags_raw = data.get("tags") or []
        return cls(
            id=snowflake_to_int(data.get("id", 0) or 0),
            channel_id=snowflake_to_int(data.get("channel_id", 0) or 0),
            thread_channel_id=snowflake_to_int(data.get("thread_channel_id", 0) or 0),
            author_id=snowflake_to_int(data.get("author_id", 0) or 0),
            title=data.get("title", "") or "",
            description=data.get("description", "") or "",
            tags=[str(t) for t in tags_raw],
            files=[BinPostFile._from_data(f) for f in files_raw],
            author_username=data.get("author_username", "") or "",
            author_avatar_url=data.get("author_avatar_url", "") or "",
            comment_count=int(data.get("comment_count", 0) or 0),
            line_comment_count=int(data.get("line_comment_count", 0) or 0),
            version_count=int(data.get("version_count", 0) or 0),
            created_at=_parse_iso8601(data.get("created_at")),
            updated_at=_parse_iso8601(data.get("updated_at")),
            state=state,
        )

    def __repr__(self) -> str:
        return f"<BinPost id={self.id} title={self.title!r} channel_id={self.channel_id}>"

    def __eq__(self, other: object) -> bool:
        return isinstance(other, BinPost) and self.id == other.id

    def __hash__(self) -> int:
        return hash(self.id)


class BinPostVersionFile:
    """
    A frozen file snapshot inside a :class:`BinPostVersion`.
    """

    __slots__ = ("id", "version_id", "filename", "language", "content", "position")

    def __init__(
        self,
        *,
        id: int,
        version_id: int,
        filename: str,
        language: str,
        content: str,
        position: int,
    ) -> None:
        self.id = id
        self.version_id = version_id
        self.filename = filename
        self.language = language
        self.content = content
        self.position = position

    @classmethod
    def _from_data(cls, data: dict) -> "BinPostVersionFile":
        return cls(
            id=snowflake_to_int(data.get("id", 0) or 0),
            version_id=snowflake_to_int(data.get("version_id", 0) or 0),
            filename=data.get("filename", "") or "",
            language=data.get("language", "") or "",
            content=data.get("content", "") or "",
            position=int(data.get("position", 0) or 0),
        )

    def __repr__(self) -> str:
        return f"<BinPostVersionFile filename={self.filename!r} version_id={self.version_id}>"


class BinPostVersion:
    """
    A frozen snapshot of a :class:`BinPost`.
    """

    __slots__ = ("id", "post_id", "version", "description", "files", "created_at")

    def __init__(
        self,
        *,
        id: int,
        post_id: int,
        version: int,
        description: str,
        files: list[BinPostVersionFile],
        created_at: Optional[datetime] = None,
    ) -> None:
        self.id = id
        self.post_id = post_id
        self.version = version
        self.description = description
        self.files = files
        self.created_at = created_at

    @classmethod
    def _from_data(cls, data: dict) -> "BinPostVersion":
        files_raw = data.get("files") or []
        return cls(
            id=snowflake_to_int(data.get("id", 0) or 0),
            post_id=snowflake_to_int(data.get("post_id", 0) or 0),
            version=int(data.get("version", 0) or 0),
            description=data.get("description", "") or "",
            files=[BinPostVersionFile._from_data(f) for f in files_raw],
            created_at=_parse_iso8601(data.get("created_at")),
        )

    def __repr__(self) -> str:
        return f"<BinPostVersion id={self.id} post_id={self.post_id} version={self.version}>"


class BinLineComment:
    """
    A comment anchored to a specific line in a file inside a post version.

    Attributes
    ----------
    parent_id:
        ID of the parent comment if this is a reply, else ``None``.
    """

    __slots__ = (
        "id",
        "post_id",
        "version_id",
        "file_id",
        "line_number",
        "author_id",
        "content",
        "parent_id",
        "author_username",
        "author_avatar_url",
        "created_at",
        "updated_at",
    )

    def __init__(
        self,
        *,
        id: int,
        post_id: int,
        version_id: int,
        file_id: int,
        line_number: int,
        author_id: int,
        content: str,
        parent_id: Optional[int] = None,
        author_username: str = "",
        author_avatar_url: str = "",
        created_at: Optional[datetime] = None,
        updated_at: Optional[datetime] = None,
    ) -> None:
        self.id = id
        self.post_id = post_id
        self.version_id = version_id
        self.file_id = file_id
        self.line_number = line_number
        self.author_id = author_id
        self.content = content
        self.parent_id = parent_id
        self.author_username = author_username
        self.author_avatar_url = author_avatar_url
        self.created_at = created_at
        self.updated_at = updated_at

    @classmethod
    def _from_data(cls, data: dict) -> "BinLineComment":
        return cls(
            id=snowflake_to_int(data.get("id", 0) or 0),
            post_id=snowflake_to_int(data.get("post_id", 0) or 0),
            version_id=snowflake_to_int(data.get("version_id", 0) or 0),
            file_id=snowflake_to_int(data.get("file_id", 0) or 0),
            line_number=int(data.get("line_number", 0) or 0),
            author_id=snowflake_to_int(data.get("author_id", 0) or 0),
            content=data.get("content", "") or "",
            parent_id=snowflake_to_int_or_none(data.get("parent_id")),
            author_username=data.get("author_username", "") or "",
            author_avatar_url=data.get("author_avatar_url", "") or "",
            created_at=_parse_iso8601(data.get("created_at")),
            updated_at=_parse_iso8601(data.get("updated_at")),
        )

    def __repr__(self) -> str:
        return (
            f"<BinLineComment id={self.id} post_id={self.post_id} "
            f"line={self.line_number}>"
        )


class BinChannelTag:
    """
    An admin-defined tag attached to a bin channel.
    """

    __slots__ = ("id", "channel_id", "name", "color")

    def __init__(self, *, id: int, channel_id: int, name: str, color: str) -> None:
        self.id = id
        self.channel_id = channel_id
        self.name = name
        self.color = color

    @classmethod
    def _from_data(cls, data: dict) -> "BinChannelTag":
        return cls(
            id=snowflake_to_int(data.get("id", 0) or 0),
            channel_id=snowflake_to_int(data.get("channel_id", 0) or 0),
            name=data.get("name", "") or "",
            color=data.get("color", "") or "",
        )

    def __repr__(self) -> str:
        return f"<BinChannelTag id={self.id} name={self.name!r}>"

    def __eq__(self, other: object) -> bool:
        return isinstance(other, BinChannelTag) and self.id == other.id

    def __hash__(self) -> int:
        return hash(self.id)
