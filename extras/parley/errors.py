"""
parley.errors
=============
Exception hierarchy for the Parley library.

All exceptions inherit from :class:`ParleyError` so callers can catch the
broad base class or be specific about which error they handle.
"""

from __future__ import annotations

from typing import Optional


__all__ = [
    "ParleyError",
    "HTTPError",
    "AuthError",
    "ForbiddenError",
    "NotFoundError",
    "RateLimitError",
    "ServerError",
    "GatewayError",
    "CommandError",
    "CommandNotFound",
    "MissingRequiredArgument",
    "BadArgument",
    "CheckFailure",
    "NotACommand",
    "ExtensionError",
    "ExtensionNotFound",
    "ExtensionAlreadyLoaded",
    "ExtensionFailed",
]


class ParleyError(Exception):
    """Base exception for all Parley errors."""


# ---------------------------------------------------------------------------
# HTTP / REST errors
# ---------------------------------------------------------------------------


class HTTPError(ParleyError):
    """Raised when the API returns an unexpected HTTP status code.

    Attributes
    ----------
    status_code:
        The HTTP status code returned by the server.
    message:
        Human-readable error message extracted from the response body.
    """

    def __init__(self, message: str, status_code: Optional[int] = None) -> None:
        super().__init__(message)
        self.status_code = status_code


class AuthError(HTTPError):
    """Raised on HTTP 401 — bad or missing authentication token."""


class ForbiddenError(HTTPError):
    """Raised on HTTP 403 — valid token but insufficient permissions."""


class NotFoundError(HTTPError):
    """Raised on HTTP 404 — the requested resource does not exist."""


class RateLimitError(HTTPError):
    """Raised on HTTP 429 — the client is being rate-limited."""


class ServerError(HTTPError):
    """Raised on HTTP 5xx — unexpected server-side error."""


# ---------------------------------------------------------------------------
# Gateway / WebSocket errors
# ---------------------------------------------------------------------------


class GatewayError(ParleyError):
    """Raised for WebSocket / gateway-related failures."""


# ---------------------------------------------------------------------------
# Command framework errors  (also re-exported from ext.commands)
# ---------------------------------------------------------------------------


class CommandError(ParleyError):
    """Base for all command-framework errors."""


class CommandNotFound(CommandError):
    """Raised when an invoked command name does not match any registered command."""

    def __init__(self, name: str) -> None:
        super().__init__(f"Command '{name}' not found.")
        self.name = name


class MissingRequiredArgument(CommandError):
    """Raised when a required argument is not provided."""

    def __init__(self, param: str) -> None:
        super().__init__(f"Missing required argument: '{param}'.")
        self.param = param


class BadArgument(CommandError):
    """Raised when an argument cannot be converted to the expected type."""


class CheckFailure(CommandError):
    """Raised when a command check predicate returns False."""


class NotACommand(CommandError):
    """Raised when a non-command object is treated as one."""


# ---------------------------------------------------------------------------
# Extension loading errors
# ---------------------------------------------------------------------------


class ExtensionError(ParleyError):
    """Base for extension (cog/module) loading errors."""

    def __init__(self, message: str, name: str) -> None:
        super().__init__(message)
        self.name = name


class ExtensionNotFound(ExtensionError):
    """Raised when the extension module cannot be imported."""

    def __init__(self, name: str) -> None:
        super().__init__(f"Extension '{name}' could not be found.", name)


class ExtensionAlreadyLoaded(ExtensionError):
    """Raised when an extension is loaded a second time without unloading first."""

    def __init__(self, name: str) -> None:
        super().__init__(f"Extension '{name}' is already loaded.", name)


class ExtensionFailed(ExtensionError):
    """Raised when an extension's setup() function raises an exception."""

    def __init__(self, name: str, original: Exception) -> None:
        super().__init__(
            f"Extension '{name}' raised an exception during setup: {original!r}",
            name,
        )
        self.original = original
