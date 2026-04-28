"""
parley.models.overwrite
=======================
:class:`Overwrite` — a per-channel permission overwrite for a role or user.

Permission overwrites layer on top of role permissions: ``allow`` bits grant
permissions in this channel, ``deny`` bits revoke them. The :class:`Permissions`
enum from :mod:`parley.enums` describes the available bits.

A target type of ``0`` denotes a role overwrite, ``1`` denotes a user
overwrite. Exactly one of :attr:`role_id` or :attr:`user_id` will be set.
"""

from __future__ import annotations

from typing import Any, Optional

from ..enums import Permissions

__all__ = ["Overwrite"]


# Backend convention: target_type 0 = role, 1 = user. Mirrors
# UpsertOverwriteRequest in internal/channel/overwrites.go.
TARGET_TYPE_ROLE = 0
TARGET_TYPE_USER = 1


class Overwrite:
    """
    A permission overwrite scoped to a single channel.

    Attributes
    ----------
    id:
        Server-side overwrite identifier (``None`` for client-built drafts).
    channel_id:
        ID of the channel this overwrite applies to.
    target_type:
        ``0`` for a role overwrite, ``1`` for a user overwrite.
    role_id:
        Role being overridden (``None`` for user overwrites).
    user_id:
        User being overridden (``None`` for role overwrites).
    allow:
        Bit-flag :class:`~parley.enums.Permissions` value granted.
    deny:
        Bit-flag :class:`~parley.enums.Permissions` value revoked.
    """

    __slots__ = (
        "id",
        "channel_id",
        "target_type",
        "role_id",
        "user_id",
        "allow",
        "deny",
    )

    def __init__(
        self,
        *,
        id: Optional[int],
        channel_id: int,
        target_type: int,
        role_id: Optional[int],
        user_id: Optional[int],
        allow: int,
        deny: int,
    ) -> None:
        self.id = id
        self.channel_id = channel_id
        self.target_type = target_type
        self.role_id = role_id
        self.user_id = user_id
        self.allow = Permissions(allow)
        self.deny = Permissions(deny)

    @classmethod
    def _from_data(cls, data: dict, state: Optional[Any] = None) -> "Overwrite":
        # Backend returns target_type + target_id; convenience role_id/user_id
        # split lets users branch without re-parsing the type.
        target_type = int(data.get("target_type", 0) or 0)
        target_id_raw = data.get("target_id")
        target_id = int(target_id_raw) if target_id_raw not in (None, "") else None

        role_id = target_id if target_type == TARGET_TYPE_ROLE else None
        user_id = target_id if target_type == TARGET_TYPE_USER else None

        raw_id = data.get("id")
        return cls(
            id=int(raw_id) if raw_id not in (None, "") else None,
            channel_id=int(data.get("channel_id", 0) or 0),
            target_type=target_type,
            role_id=role_id,
            user_id=user_id,
            allow=int(data.get("allow", 0) or 0),
            deny=int(data.get("deny", 0) or 0),
        )

    @property
    def target_id(self) -> Optional[int]:
        """ID of the role or user this overwrite targets."""
        return self.role_id if self.target_type == TARGET_TYPE_ROLE else self.user_id

    def __repr__(self) -> str:
        kind = "role" if self.target_type == TARGET_TYPE_ROLE else "user"
        return (
            f"<Overwrite channel_id={self.channel_id} {kind}_id={self.target_id} "
            f"allow={int(self.allow)} deny={int(self.deny)}>"
        )

    def __eq__(self, other: object) -> bool:
        return (
            isinstance(other, Overwrite)
            and self.channel_id == other.channel_id
            and self.target_type == other.target_type
            and self.target_id == other.target_id
        )

    def __hash__(self) -> int:
        return hash((self.channel_id, self.target_type, self.target_id))
