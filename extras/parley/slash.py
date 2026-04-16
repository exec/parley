"""
parley.slash
============
Slash-command declarations and runtime dispatch.

This module provides:

- :class:`SlashOption`   — declarative description of a command option.
- :class:`SlashCommand`  — a registered slash command (name, description,
  options, handler).
- :class:`InteractionContext` — the runtime object passed to a slash-command
  handler, with a :meth:`~InteractionContext.respond` helper.

The :class:`~parley.client.Bot` class owns a registry of commands and a
``slash_command()`` decorator; see :mod:`parley.client` for registration.

Typical usage::

    from parley import Bot, SlashOption

    bot = Bot("https://parley.x86-64.com", api_key="plk_...")

    @bot.slash_command(
        name="weather",
        description="Current weather",
        options=[
            SlashOption("city", "City name", type="STRING", required=True),
        ],
    )
    async def weather(ctx, city: str):
        await ctx.respond(f"Weather in {city}: sunny")

    bot.run()

Only ``STRING``, ``INTEGER``, and ``BOOLEAN`` option types are supported in
v1.  No USER/CHANNEL/ROLE yet.
"""

from __future__ import annotations

import logging
import re
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any, Awaitable, Callable, Optional

if TYPE_CHECKING:
    from .client import Client

__all__ = [
    "SlashOption",
    "SlashCommand",
    "InteractionContext",
    "VALID_OPTION_TYPES",
]

log = logging.getLogger("parley.slash")

# ---------------------------------------------------------------------------
# Validation constants
# ---------------------------------------------------------------------------

VALID_OPTION_TYPES: frozenset[str] = frozenset({"STRING", "INTEGER", "BOOLEAN"})

# Slash-command name rules: 1-32 chars, lowercase letters/digits/_/-
_NAME_RE = re.compile(r"^[a-z0-9_-]{1,32}$")

_MAX_NAME_LEN = 32
_MAX_DESC_LEN = 100
_MAX_OPTIONS = 25
_MAX_CHOICES = 25


def _validate_name(name: str, *, label: str = "name") -> None:
    if not isinstance(name, str) or not name:
        raise ValueError(f"{label} must be a non-empty string")
    if len(name) > _MAX_NAME_LEN:
        raise ValueError(
            f"{label} must be at most {_MAX_NAME_LEN} characters (got {len(name)})"
        )
    if not _NAME_RE.match(name):
        raise ValueError(
            f"{label} {name!r} must be lowercase and match {_NAME_RE.pattern}"
        )


def _validate_description(desc: str, *, label: str = "description") -> None:
    if not isinstance(desc, str) or not desc:
        raise ValueError(f"{label} must be a non-empty string")
    if len(desc) > _MAX_DESC_LEN:
        raise ValueError(
            f"{label} must be at most {_MAX_DESC_LEN} characters (got {len(desc)})"
        )


def _normalize_choice(choice: Any) -> dict:
    """Accept either (name, value) tuple/list or {'name': ..., 'value': ...} dict."""
    if isinstance(choice, dict):
        if "name" not in choice or "value" not in choice:
            raise ValueError(
                "choice dicts must contain 'name' and 'value' keys"
            )
        return {"name": str(choice["name"]), "value": choice["value"]}
    if isinstance(choice, (list, tuple)) and len(choice) == 2:
        return {"name": str(choice[0]), "value": choice[1]}
    raise ValueError(
        f"choice {choice!r} must be a (name, value) tuple or "
        "{'name': ..., 'value': ...} dict"
    )


# ---------------------------------------------------------------------------
# SlashOption
# ---------------------------------------------------------------------------


@dataclass
class SlashOption:
    """
    Declarative description of a single slash-command option.

    Parameters
    ----------
    name:
        Option name (1-32 chars, lowercase).
    description:
        Short description shown to the user (1-100 chars).
    type:
        One of ``"STRING"``, ``"INTEGER"``, ``"BOOLEAN"``.
    required:
        Whether the option must be supplied.
    choices:
        Optional list of predefined choices.  Each item may be either a
        ``(name, value)`` tuple or a ``{"name": ..., "value": ...}`` dict.
    min_value, max_value:
        Optional numeric bounds (for ``INTEGER``).
    min_length, max_length:
        Optional string-length bounds (for ``STRING``).
    """

    name: str
    description: str
    type: str
    required: bool = False
    choices: Optional[list] = None
    min_value: Optional[float] = None
    max_value: Optional[float] = None
    min_length: Optional[int] = None
    max_length: Optional[int] = None

    def __post_init__(self) -> None:
        _validate_name(self.name, label="option name")
        _validate_description(self.description, label="option description")
        if self.type not in VALID_OPTION_TYPES:
            raise ValueError(
                f"option type must be one of {sorted(VALID_OPTION_TYPES)}, "
                f"got {self.type!r}"
            )
        if self.choices is not None:
            if len(self.choices) > _MAX_CHOICES:
                raise ValueError(
                    f"an option may have at most {_MAX_CHOICES} choices "
                    f"(got {len(self.choices)})"
                )
            # Validate each choice shape eagerly; we don't mutate here to
            # preserve the caller's original list.
            for c in self.choices:
                _normalize_choice(c)

    def to_api(self) -> dict:
        """Serialize to the JSON shape expected by the backend."""
        body: dict[str, Any] = {
            "name": self.name,
            "description": self.description,
            "type": self.type,
            "required": bool(self.required),
        }
        if self.choices is not None:
            body["choices"] = [_normalize_choice(c) for c in self.choices]
        if self.min_value is not None:
            body["min_value"] = self.min_value
        if self.max_value is not None:
            body["max_value"] = self.max_value
        if self.min_length is not None:
            body["min_length"] = self.min_length
        if self.max_length is not None:
            body["max_length"] = self.max_length
        return body


# ---------------------------------------------------------------------------
# SlashCommand
# ---------------------------------------------------------------------------


@dataclass
class SlashCommand:
    """
    A registered slash command.

    The handler signature must be::

        async def handler(ctx: InteractionContext, **option_kwargs) -> None

    Options from the invocation are passed by keyword; optional options that
    were not supplied by the invoker are omitted from ``option_kwargs``.
    """

    name: str
    description: str
    options: list = field(default_factory=list)
    handler: Optional[Callable[..., Awaitable[Any]]] = None

    def __post_init__(self) -> None:
        _validate_name(self.name, label="command name")
        _validate_description(self.description, label="command description")
        if not isinstance(self.options, list):
            raise ValueError("options must be a list of SlashOption")
        if len(self.options) > _MAX_OPTIONS:
            raise ValueError(
                f"a command may have at most {_MAX_OPTIONS} options "
                f"(got {len(self.options)})"
            )
        seen: set[str] = set()
        required_done = False
        for opt in self.options:
            if not isinstance(opt, SlashOption):
                raise ValueError(
                    "each option must be a SlashOption instance, "
                    f"got {type(opt).__name__}"
                )
            if opt.name in seen:
                raise ValueError(f"duplicate option name {opt.name!r}")
            seen.add(opt.name)
            # Required options must come before optional ones.
            if required_done and opt.required:
                raise ValueError(
                    "required options must be declared before optional ones "
                    f"(offender: {opt.name!r})"
                )
            if not opt.required:
                required_done = True

    def to_api(self) -> dict:
        """Serialize to the JSON shape the backend PUT/POST endpoints expect."""
        return {
            "name": self.name,
            "description": self.description,
            "options": [o.to_api() for o in self.options],
        }


# ---------------------------------------------------------------------------
# InteractionContext
# ---------------------------------------------------------------------------


class InteractionContext:
    """
    Runtime context passed to a slash-command handler.

    Attributes
    ----------
    token:
        Opaque interaction token (used as auth for
        ``POST /api/interactions/:token/respond``).
    command_name:
        Name of the invoked command.
    command_id:
        Backend ID of the command.
    channel_id, server_id:
        IDs of the invocation location.
    invoker:
        Dict with keys ``id``, ``username``, ``display_name``, ``avatar_url``.
    options:
        Mapping of option name → value as supplied by the invoker.  Optional
        options the user didn't supply are absent from this dict.
    """

    __slots__ = (
        "_client",
        "_payload",
        "token",
        "command_name",
        "command_id",
        "channel_id",
        "server_id",
        "invoker",
        "options",
        "created_at",
        "expires_at",
        "_responded",
    )

    def __init__(self, client: "Client", payload: dict) -> None:
        self._client = client
        self._payload = payload

        self.token: str = str(payload["token"])
        cmd = payload.get("command") or {}
        self.command_name: str = str(cmd.get("name", ""))
        try:
            self.command_id: int = int(cmd.get("id", 0))
        except (TypeError, ValueError):
            self.command_id = 0

        try:
            self.channel_id: int = int(payload.get("channel_id", 0))
        except (TypeError, ValueError):
            self.channel_id = 0
        try:
            self.server_id: int = int(payload.get("server_id", 0))
        except (TypeError, ValueError):
            self.server_id = 0

        self.invoker: dict = payload.get("invoker") or {}
        self.options: dict = dict(payload.get("options") or {})
        self.created_at: Optional[str] = payload.get("created_at")
        self.expires_at: Optional[str] = payload.get("expires_at")
        self._responded = False

    # ------------------------------------------------------------------
    # Properties
    # ------------------------------------------------------------------

    @property
    def responded(self) -> bool:
        """Whether :meth:`respond` has already been called for this interaction."""
        return self._responded

    @property
    def bot(self) -> "Client":
        """The owning :class:`~parley.client.Client` / :class:`Bot`."""
        return self._client

    # ------------------------------------------------------------------
    # Response
    # ------------------------------------------------------------------

    async def respond(self, content: str) -> dict:
        """
        Respond to the interaction.

        Issues ``POST /api/interactions/:token/respond``.  The interaction
        token in the URL path is the authentication — this call does **not**
        send the bot's normal ``Authorization`` header.

        Parameters
        ----------
        content:
            The text content to send as the response message.

        Returns
        -------
        dict
            The raw JSON response from the backend (typically
            ``{"message_id": ...}``).

        Raises
        ------
        RuntimeError
            If :meth:`respond` has already been called for this interaction.
        parley.HTTPError
            On any non-2xx response.  Status ``409`` means the interaction is
            no longer pending; ``410`` means it has expired.
        """
        if self._responded:
            raise RuntimeError("Already responded to this interaction")
        if not isinstance(content, str):
            raise TypeError("content must be a str")

        path = f"/api/interactions/{self.token}/respond"
        body = {"content": content}

        # The respond endpoint authenticates off the token in the URL, not
        # the bot's Authorization header. We still go through the shared
        # httpx.AsyncClient for connection pooling, but we explicitly blank
        # the Authorization header for this request so the backend treats
        # the call as token-authed (and so selfbot JWTs aren't leaked here).
        http = self._client.http
        client = await http._ensure_client()
        try:
            resp = await client.request(
                "POST",
                path,
                json=body,
                headers={"Authorization": ""},
            )
        except Exception:
            log.exception(
                "Interaction respond failed for token=%s…", self.token[:8]
            )
            raise

        # Re-use the HTTPClient's shared error handling so bot authors get
        # the same exception hierarchy as the rest of the SDK.
        from .http import _raise_for_status
        _raise_for_status(resp)

        self._responded = True

        if resp.status_code == 204 or not resp.content:
            return {}
        try:
            return resp.json()
        except Exception:
            return {}

    def __repr__(self) -> str:
        return (
            f"<InteractionContext command={self.command_name!r} "
            f"channel_id={self.channel_id} server_id={self.server_id}>"
        )
