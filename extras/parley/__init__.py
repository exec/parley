"""
parley
======
Async Python library for the Parley chat platform.

Supports bot (API key) and selfbot (JWT token) usage with a full command
framework, gateway event system, and model layer.

Quick start — bot with commands::

    import parley

    bot = parley.CommandBot(
        "https://parley.x86-64.com",
        api_key="plk_...",
        command_prefix="!",
    )

    @bot.command()
    async def ping(ctx):
        await ctx.reply("pong!")

    @bot.event
    async def on_message_create(message):
        if "hello" in message.content.lower():
            await message.reply("Hi there!")

    bot.run()

Quick start — selfbot::

    import parley

    sb = parley.Selfbot("https://parley.x86-64.com", token="eyJ...")

    @sb.event
    async def on_message_create(message):
        if message.mentions_user(sb.user.id):
            await message.reply("I'm away right now!")

    sb.run()

Cog example::

    class MyCog(parley.Cog):
        @parley.command()
        async def hello(self, ctx):
            await ctx.send(f"Hello {ctx.author.display}!")

        @parley.Cog.listener()
        async def on_message_create(self, message):
            print(message.content)

    bot.add_cog(MyCog(bot))
"""

from __future__ import annotations

# -- Version -----------------------------------------------------------------
__version__ = "1.1.0"
__author__ = "Parley Contributors"

# -- Client classes ----------------------------------------------------------
from .client import Bot, Client, Selfbot

# CommandBot is in ext but exposed at the top level for convenience.
from .ext.commands.bot import CommandBot

# -- Models ------------------------------------------------------------------
from .models.channel import BinChannel, Channel, TextChannel, VoiceChannel
from .models.dm import DmChannel
from .models.invite import Invite
from .models.member import Member
from .models.message import DmMessage, Message
from .models.role import Role
from .models.server import Server
from .models.user import ClientUser, PublicUser, User

# -- Enums -------------------------------------------------------------------
from .enums import Badges, ChannelType, Permissions

# -- Errors ------------------------------------------------------------------
from .errors import (
    AuthError,
    BadArgument,
    CheckFailure,
    CommandError,
    CommandNotFound,
    ExtensionAlreadyLoaded,
    ExtensionError,
    ExtensionFailed,
    ExtensionNotFound,
    ForbiddenError,
    GatewayError,
    HTTPError,
    MissingRequiredArgument,
    NotACommand,
    NotFoundError,
    ParleyError,
    RateLimitError,
    ServerError,
)

# -- Command framework re-exports --------------------------------------------
from .ext.commands.cog import Cog
from .ext.commands.context import Context
from .ext.commands.core import Command, Group, check, command, group

# -- Slash-command framework -------------------------------------------------
from .slash import InteractionContext, SlashCommand, SlashOption

# -- Utils -------------------------------------------------------------------
from .utils import MISSING, format_mention, parse_mentions, snowflake_time, strip_mention

# -- Friend / DM-group / Notification / Member-role models -------------------
from .models.friend import Friend, FriendRequest
from .models.notification import Notification

# -- Uploads / Passkey / GDPR / Overwrites / Message-extras models -----------
from .models.overwrite import Overwrite
from .models.passkey import Passkey

# -- Voice / Soundboard / Theme / Bin models ---------------------------------
from .models.bin import (
    BinChannelTag,
    BinLineComment,
    BinPost,
    BinPostFile,
    BinPostVersion,
    BinPostVersionFile,
)
from .models.soundboard import Sound
from .models.theme import ThemePreferences, UserTheme
from .models.voice import ActiveCalls, Ring, VoiceParticipant, VoiceToken

__all__ = [
    # Version
    "__version__",
    # Clients
    "Client",
    "Bot",
    "Selfbot",
    "CommandBot",
    # Models
    "User",
    "ClientUser",
    "PublicUser",
    "Server",
    "Channel",
    "TextChannel",
    "VoiceChannel",
    "BinChannel",
    "Member",
    "Role",
    "Message",
    "DmMessage",
    "DmChannel",
    "Invite",
    # Enums
    "ChannelType",
    "Badges",
    "Permissions",
    # Errors
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
    # Command framework
    "Cog",
    "Context",
    "Command",
    "Group",
    "command",
    "group",
    "check",
    # Slash-command framework
    "SlashOption",
    "SlashCommand",
    "InteractionContext",
    # Utils
    "MISSING",
    "snowflake_time",
    "parse_mentions",
    "strip_mention",
    "format_mention",
    # Friend / Notification
    "Friend",
    "FriendRequest",
    "Notification",
    # Passkey / Overwrite
    "Overwrite",
    "Passkey",
    # Voice
    "VoiceToken",
    "VoiceParticipant",
    "Ring",
    "ActiveCalls",
    # Soundboard
    "Sound",
    # Theme
    "UserTheme",
    "ThemePreferences",
    # Bin
    "BinPost",
    "BinPostFile",
    "BinPostVersion",
    "BinPostVersionFile",
    "BinLineComment",
    "BinChannelTag",
]
