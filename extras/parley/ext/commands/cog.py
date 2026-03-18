"""
parley.ext.commands.cog
=======================
:class:`Cog` — a container for grouping related commands and listeners.

Cogs provide a clean way to organise bot functionality into logical units
that can be loaded and unloaded at runtime.

Example::

    class Moderation(parley.Cog):
        @parley.command()
        async def kick(self, ctx, user_id: int):
            await ctx.server.kick(user_id)
            await ctx.send(f"Kicked <@{user_id}>.")

        @parley.Cog.listener()
        async def on_message_create(self, message):
            if "bad word" in message.content.lower():
                await message.delete()

    bot.add_cog(Moderation(bot))
"""

from __future__ import annotations

import asyncio
import inspect
from typing import TYPE_CHECKING, Any, Callable, Optional

from .core import Command, Group

if TYPE_CHECKING:
    from .bot import CommandBot

__all__ = ["Cog"]

_LISTENER_ATTR = "__cog_listener__"
_LISTENER_EVENTS_ATTR = "__cog_listener_events__"


class CogMeta(type):
    """Metaclass that collects commands and listeners defined in a :class:`Cog` body."""

    def __new__(mcs, name: str, bases: tuple, namespace: dict, **kwargs: Any) -> "CogMeta":
        commands: list[Command] = []
        listeners: list[tuple[str, Callable]] = []

        for key, value in namespace.items():
            if isinstance(value, Command):
                commands.append(value)
            elif asyncio.iscoroutinefunction(value) and getattr(
                value, _LISTENER_ATTR, False
            ):
                for event_name in getattr(value, _LISTENER_EVENTS_ATTR, [key]):
                    listeners.append((event_name, value))

        namespace["__cog_commands__"] = commands
        namespace["__cog_listeners__"] = listeners
        return super().__new__(mcs, name, bases, namespace)


class Cog(metaclass=CogMeta):
    """
    Base class for command cogs.

    Subclass this and use :meth:`listener` to register event handlers and
    define commands with :func:`~parley.ext.commands.core.command` (or
    :func:`~parley.ext.commands.core.group`) decorators.

    Parameters
    ----------
    bot:
        The :class:`~parley.ext.commands.bot.CommandBot` this cog is
        attached to.  Accessible via ``self.bot`` after :meth:`_inject`.
    """

    __cog_commands__: list[Command]
    __cog_listeners__: list[tuple[str, Callable]]

    def __init__(self, bot: Optional["CommandBot"] = None) -> None:
        self.bot = bot

    # ------------------------------------------------------------------
    # Listener decorator
    # ------------------------------------------------------------------

    @staticmethod
    def listener(name: Optional[str] = None) -> Callable:
        """Decorator that marks a cog method as an event listener.

        Parameters
        ----------
        name:
            Event name (without ``on_`` prefix).  If omitted, the method
            name is used after stripping ``on_``.

        Example::

            @Cog.listener()
            async def on_message_create(self, message):
                ...

            @Cog.listener("message_create")
            async def handle_messages(self, message):
                ...
        """

        def decorator(fn: Callable) -> Callable:
            setattr(fn, _LISTENER_ATTR, True)
            event_name = name or fn.__name__
            if event_name.startswith("on_"):
                event_name = event_name[3:]
            setattr(fn, _LISTENER_EVENTS_ATTR, [event_name])
            return fn

        return decorator

    # ------------------------------------------------------------------
    # Internal injection (called by CommandBot.add_cog)
    # ------------------------------------------------------------------

    def _inject(self, bot: "CommandBot") -> None:
        """Bind this cog to *bot*, attaching commands and listeners."""
        self.bot = bot
        for cmd in self.__cog_commands__:
            cmd.cog = self
            bot.add_command(cmd)

        for event_name, method in self.__cog_listeners__:
            # Bind the method to this cog instance
            bound = method.__get__(self, type(self))
            bot.add_listener(bound, event_name)

    def _eject(self, bot: "CommandBot") -> None:
        """Remove this cog's commands and listeners from *bot*."""
        for cmd in self.__cog_commands__:
            bot.remove_command(cmd.name)
            cmd.cog = None

        for event_name, method in self.__cog_listeners__:
            bound = method.__get__(self, type(self))
            bot.remove_listener(bound, event_name)
        self.bot = None

    # ------------------------------------------------------------------
    # Optional lifecycle hooks
    # ------------------------------------------------------------------

    async def cog_load(self) -> None:
        """Called when the cog is added to the bot via :meth:`CommandBot.add_cog`.

        Override to perform async setup (e.g. database connections).
        """

    async def cog_unload(self) -> None:
        """Called when the cog is removed from the bot.

        Override to perform async teardown.
        """

    def __repr__(self) -> str:
        return f"<Cog {type(self).__name__}>"
