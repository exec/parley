"""
parley.ext.commands.bot
=======================
:class:`CommandBot` — a :class:`~parley.client.Bot` with a full command
framework, cog support, and extension loading.

Typical usage::

    bot = parley.CommandBot(
        "https://parley.x86-64.com",
        api_key="plk_...",
        command_prefix="!",
    )

    @bot.command()
    async def ping(ctx):
        await ctx.reply("pong!")

    bot.run()
"""

from __future__ import annotations

import importlib
import importlib.util
import logging
import sys
from typing import Any, Callable, Coroutine, Optional, TypeVar

from parley.client import Bot
from parley.errors import (
    CommandNotFound,
    ExtensionAlreadyLoaded,
    ExtensionFailed,
    ExtensionNotFound,
)
from parley.models.message import Message

from .cog import Cog
from .context import Context
from .core import Command, Group, command as _command_decorator
from .errors import CommandError

__all__ = ["CommandBot"]

log = logging.getLogger("parley.commands")

_CoroFunc = Callable[..., Coroutine[Any, Any, Any]]
T = TypeVar("T", bound=_CoroFunc)

_PrefixType = str | list[str] | Callable


class CommandBot(Bot):
    """
    A :class:`~parley.client.Bot` extended with a command framework.

    Parameters
    ----------
    base_url:
        Root URL of the Parley instance.
    api_key:
        Developer API key.
    command_prefix:
        String, list of strings, or ``async def prefix(bot, message) -> str``
        callable.  When a list, the first matching prefix is used.
    help_command:
        Set to ``False`` to disable the built-in ``!help`` command
        (or provide a custom :class:`Command` instance).
    timeout:
        HTTP timeout in seconds.
    """

    def __init__(
        self,
        base_url: str,
        *,
        api_key: str,
        command_prefix: _PrefixType = "!",
        help_command: Any = True,
        timeout: float = 30.0,
    ) -> None:
        super().__init__(base_url, api_key=api_key, timeout=timeout)
        self.command_prefix = command_prefix
        self._commands: dict[str, Command] = {}
        self._cogs: dict[str, Cog] = {}
        self._extensions: dict[str, Any] = {}

        # Auto-register message_create to run command parsing
        self.add_listener(self._process_commands, "message_create")

        if help_command is True:
            self._register_default_help()

    # ------------------------------------------------------------------
    # Command registration
    # ------------------------------------------------------------------

    def add_command(self, cmd: Command) -> None:
        """Register a :class:`~parley.ext.commands.core.Command` on this bot.

        Parameters
        ----------
        cmd:
            Command instance to register.
        """
        for name in cmd.all_names():
            if name in self._commands:
                log.warning("Overwriting existing command %r", name)
            self._commands[name] = cmd

    def remove_command(self, name: str) -> Optional[Command]:
        """Remove a command by name.  Returns the removed command or ``None``."""
        cmd = self._commands.pop(name, None)
        if cmd:
            for alias in cmd.aliases:
                self._commands.pop(alias, None)
        return cmd

    def get_command(self, name: str) -> Optional[Command]:
        """Retrieve a registered command by name or alias."""
        return self._commands.get(name)

    def command(
        self,
        name: Optional[str] = None,
        *,
        aliases: Optional[list[str]] = None,
        help: Optional[str] = None,
    ) -> Callable[[T], Command]:
        """Decorator that registers a command on this bot.

        Example::

            @bot.command()
            async def ping(ctx):
                await ctx.reply("pong!")
        """

        def decorator(fn: T) -> Command:
            cmd = Command(fn, name=name, aliases=aliases, help=help)
            self.add_command(cmd)
            return cmd

        return decorator  # type: ignore[return-value]

    def group(
        self,
        name: Optional[str] = None,
        *,
        aliases: Optional[list[str]] = None,
        help: Optional[str] = None,
    ) -> Callable[[T], Group]:
        """Decorator that registers a :class:`~parley.ext.commands.core.Group`."""

        def decorator(fn: T) -> Group:
            grp = Group(fn, name=name, aliases=aliases, help=help)
            self.add_command(grp)
            return grp

        return decorator  # type: ignore[return-value]

    # ------------------------------------------------------------------
    # Cog management
    # ------------------------------------------------------------------

    def add_cog(self, cog: Cog) -> None:
        """Add a :class:`~parley.ext.commands.cog.Cog` to the bot.

        Injects the cog's commands and listeners, then calls
        :meth:`~parley.ext.commands.cog.Cog.cog_load`.

        Parameters
        ----------
        cog:
            An instance of a :class:`Cog` subclass.
        """
        import asyncio

        name = type(cog).__name__
        self._cogs[name] = cog
        cog._inject(self)
        try:
            loop = asyncio.get_event_loop()
            if loop.is_running():
                asyncio.ensure_future(cog.cog_load())
        except RuntimeError:
            pass  # No event loop yet; cog_load will be skipped silently

    def remove_cog(self, name: str) -> Optional[Cog]:
        """Remove a cog by class name.

        Parameters
        ----------
        name:
            The class name of the cog (e.g. ``"Moderation"``).

        Returns
        -------
        :class:`~parley.ext.commands.cog.Cog` or ``None``
        """
        import asyncio

        cog = self._cogs.pop(name, None)
        if cog is not None:
            cog._eject(self)
            try:
                loop = asyncio.get_event_loop()
                if loop.is_running():
                    asyncio.ensure_future(cog.cog_unload())
            except RuntimeError:
                pass
        return cog

    def get_cog(self, name: str) -> Optional[Cog]:
        """Retrieve a loaded cog by class name."""
        return self._cogs.get(name)

    # ------------------------------------------------------------------
    # Extension loading
    # ------------------------------------------------------------------

    def load_extension(self, name: str) -> None:
        """Import a Python module and call its ``setup(bot)`` function.

        The module must define a top-level ``async def setup(bot)`` or
        ``def setup(bot)`` function.

        Parameters
        ----------
        name:
            Dotted module path, e.g. ``"cogs.moderation"``.

        Raises
        ------
        :exc:`~parley.errors.ExtensionAlreadyLoaded`
            If the extension is already loaded.
        :exc:`~parley.errors.ExtensionNotFound`
            If the module cannot be found.
        :exc:`~parley.errors.ExtensionFailed`
            If ``setup()`` raises an exception.
        """
        if name in self._extensions:
            raise ExtensionAlreadyLoaded(name)

        try:
            module = importlib.import_module(name)
        except ModuleNotFoundError as exc:
            raise ExtensionNotFound(name) from exc

        setup = getattr(module, "setup", None)
        if setup is None:
            raise ExtensionFailed(
                name, RuntimeError(f"Module '{name}' has no 'setup' function")
            )

        try:
            import asyncio
            result = setup(self)
            if asyncio.iscoroutine(result):
                try:
                    loop = asyncio.get_event_loop()
                    if loop.is_running():
                        asyncio.ensure_future(result)
                    else:
                        loop.run_until_complete(result)
                except RuntimeError:
                    pass
        except Exception as exc:
            raise ExtensionFailed(name, exc) from exc

        self._extensions[name] = module

    def unload_extension(self, name: str) -> None:
        """Unload a previously loaded extension.

        Calls ``teardown(bot)`` if defined, then removes the module from
        ``sys.modules``.

        Parameters
        ----------
        name:
            Dotted module path of the extension to unload.
        """
        module = self._extensions.pop(name, None)
        if module is None:
            return

        teardown = getattr(module, "teardown", None)
        if teardown is not None:
            try:
                import asyncio
                result = teardown(self)
                if asyncio.iscoroutine(result):
                    asyncio.ensure_future(result)
            except Exception:
                log.exception("Error in teardown for extension %r", name)

        sys.modules.pop(name, None)

    def reload_extension(self, name: str) -> None:
        """Unload then reload an extension."""
        self.unload_extension(name)
        self.load_extension(name)

    # ------------------------------------------------------------------
    # Command processing
    # ------------------------------------------------------------------

    async def _get_prefix(self, message: Message) -> list[str]:
        """Resolve the command prefix for *message*.

        Returns a list of possible prefix strings.
        """
        prefix = self.command_prefix
        if callable(prefix):
            import asyncio
            result = prefix(self, message)
            if asyncio.iscoroutine(result):
                result = await result
            prefix = result

        if isinstance(prefix, str):
            return [prefix]
        return list(prefix)

    async def _process_commands(self, message: Message) -> None:
        """Parse *message* for a command invocation and dispatch it.

        Called automatically for every ``MESSAGE_CREATE`` event.
        Ignores messages sent by the bot itself.
        """
        if self._state.user and message.author_id == self._state.user.id:
            return

        prefixes = await self._get_prefix(message)

        content = message.content
        used_prefix: Optional[str] = None
        for p in prefixes:
            if content.startswith(p):
                used_prefix = p
                content = content[len(p):]
                break

        if used_prefix is None:
            return

        parts = content.split()
        if not parts:
            return

        cmd_name = parts[0]
        cmd = self._commands.get(cmd_name)

        if cmd is None:
            await self.on_command_not_found(message, cmd_name)
            return

        ctx = Context(
            message=message,
            bot=self,
            command=cmd,
            args=parts[1:],
            prefix=used_prefix,
            invoked_with=cmd_name,
        )

        await self.invoke(ctx)

    async def invoke(self, ctx: Context) -> None:
        """Invoke a command from a fully constructed :class:`Context`.

        Parameters
        ----------
        ctx:
            Ready-to-use context object.
        """
        if ctx.command is None:
            return
        try:
            await ctx.command.invoke(ctx)
        except CommandError as exc:
            await self.on_command_error(ctx, exc)
        except Exception as exc:
            await self.on_command_error(ctx, exc)

    # ------------------------------------------------------------------
    # Error / not-found hooks (override in subclass)
    # ------------------------------------------------------------------

    async def on_command_error(self, ctx: Context, error: Exception) -> None:
        """Called when a command raises an exception.

        Override to provide custom error handling::

            async def on_command_error(self, ctx, error):
                await ctx.send(f"Error: {error}")

        Default behaviour: logs the error.
        """
        log.error(
            "Command '%s' raised %s: %s",
            ctx.invoked_with,
            type(error).__name__,
            error,
        )

    async def on_command_not_found(self, message: Message, name: str) -> None:
        """Called when a prefixed message doesn't match any command.

        Override to customise this behaviour (e.g. suggest similar commands).
        Default: silently ignore.
        """

    # ------------------------------------------------------------------
    # Built-in help command
    # ------------------------------------------------------------------

    def _register_default_help(self) -> None:
        bot = self

        async def _help(ctx: Context) -> None:
            """Show available commands."""
            seen: set[str] = set()
            lines = ["**Available commands:**"]
            for name, cmd in sorted(bot._commands.items()):
                if cmd.name in seen:
                    continue
                seen.add(cmd.name)
                aliases = f" (aliases: {', '.join(cmd.aliases)})" if cmd.aliases else ""
                brief = f" — {cmd.brief}" if cmd.brief else ""
                lines.append(f"• `{ctx.prefix}{cmd.name}`{aliases}{brief}")

            await ctx.send("\n".join(lines))

        help_cmd = Command(_help, name="help", help="Show available commands.")
        self.add_command(help_cmd)
