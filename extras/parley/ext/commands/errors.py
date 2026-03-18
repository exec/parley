"""
parley.ext.commands.errors
==========================
Command-framework specific exceptions.

All are re-exported from :mod:`parley.errors` as well; this module exists
so users can do ``from parley.ext.commands import errors`` or import
directly from ``parley.ext.commands``.
"""

from parley.errors import (
    BadArgument,
    CheckFailure,
    CommandError,
    CommandNotFound,
    MissingRequiredArgument,
    NotACommand,
)

__all__ = [
    "CommandError",
    "CommandNotFound",
    "MissingRequiredArgument",
    "BadArgument",
    "CheckFailure",
    "NotACommand",
]
