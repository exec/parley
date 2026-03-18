"""
parley.ext.commands
===================
The Parley command framework.

Provides :class:`CommandBot`, :class:`Cog`, decorators, and context
classes for building feature-rich bots with a command prefix system.

Quick start::

    import parley

    bot = parley.CommandBot("https://...", api_key="plk_...", command_prefix="!")

    @bot.command()
    async def ping(ctx):
        await ctx.reply("pong!")

    bot.run()
"""

from .bot import CommandBot
from .cog import Cog
from .context import Context
from .core import Command, Group, check, command, group
from .errors import (
    BadArgument,
    CheckFailure,
    CommandError,
    CommandNotFound,
    MissingRequiredArgument,
    NotACommand,
)

__all__ = [
    # Bot
    "CommandBot",
    # Cog
    "Cog",
    # Context
    "Context",
    # Core
    "Command",
    "Group",
    "command",
    "group",
    "check",
    # Errors
    "CommandError",
    "CommandNotFound",
    "MissingRequiredArgument",
    "BadArgument",
    "CheckFailure",
    "NotACommand",
]
