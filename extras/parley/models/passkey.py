"""
parley.models.passkey
=====================
:class:`Passkey` — a WebAuthn credential registered to the authenticated user.

Returned by :meth:`parley.http.HTTPClient.list_passkeys`. Used for surfacing
the user's registered authenticators (name, last-used time) in account UIs.
The library does not perform WebAuthn ceremony itself; ``register/begin`` and
``register/finish`` return raw option/credential dicts that a browser-side
``navigator.credentials.create()`` consumer is responsible for.
"""

from __future__ import annotations

from typing import Any, Optional

__all__ = ["Passkey"]


class Passkey:
    """
    A WebAuthn passkey registered to the authenticated user.

    Attributes
    ----------
    id:
        Server-side identifier of this passkey (used for rename/delete).
    name:
        User-supplied label for the credential (e.g. ``"YubiKey 5"``).
    last_used_at:
        ISO-8601 timestamp of the most recent successful login, or ``None``
        if the credential has never been used.
    created_at:
        ISO-8601 timestamp of when the passkey was registered.
    """

    __slots__ = ("id", "name", "last_used_at", "created_at")

    def __init__(
        self,
        *,
        id: str,
        name: str,
        last_used_at: Optional[str],
        created_at: Optional[str],
    ) -> None:
        self.id = id
        self.name = name
        self.last_used_at = last_used_at
        self.created_at = created_at

    @classmethod
    def _from_data(cls, data: dict, state: Optional[Any] = None) -> "Passkey":
        return cls(
            id=str(data.get("id", "")),
            name=data.get("name", "") or "",
            last_used_at=data.get("last_used_at"),
            created_at=data.get("created_at"),
        )

    def __repr__(self) -> str:
        return f"<Passkey id={self.id!r} name={self.name!r}>"

    def __eq__(self, other: object) -> bool:
        return isinstance(other, Passkey) and self.id == other.id

    def __hash__(self) -> int:
        return hash(self.id)
