"""
parley.ext.commands.core
========================
Core building blocks of the command framework.

- :class:`Command`  — wraps a coroutine as a command.
- :class:`Group`    — a command that has sub-commands.
- :func:`command`   — decorator to create a :class:`Command`.
- :func:`group`     — decorator to create a :class:`Group`.
- :func:`check`     — decorator to add a predicate check to a command.
"""

from __future__ import annotations

import asyncio
import inspect
from typing import Any, Callable, Coroutine, Optional, TypeVar

from .errors import BadArgument, CheckFailure, MissingRequiredArgument

__all__ = [
    "Command",
    "Group",
    "command",
    "group",
    "check",
]

_CoroFunc = Callable[..., Coroutine[Any, Any, Any]]
T = TypeVar("T", bound=_CoroFunc)


class Command:
    """
    Represents a bot command.

    Attributes
    ----------
    name:
        The primary name of the command.
    callback:
        The async function to invoke.
    aliases:
        Alternative names for the command.
    help:
        Help text (defaults to the function's docstring).
    brief:
        One-line description (first line of *help*).
    cog:
        The :class:`~parley.ext.commands.cog.Cog` this command belongs to
        (set by :meth:`~parley.ext.commands.cog.Cog._inject`).
    checks:
        List of predicate callables added via :func:`check`.
    """

    def __init__(
        self,
        callback: _CoroFunc,
        *,
        name: Optional[str] = None,
        aliases: Optional[list[str]] = None,
        help: Optional[str] = None,
    ) -> None:
        if not asyncio.iscoroutinefunction(callback):
            raise TypeError(f"Command callback {callback.__name__!r} must be a coroutine")
        self.callback = callback
        self.name: str = name or callback.__name__
        self.aliases: list[str] = aliases or []
        self.help: str = help or inspect.getdoc(callback) or ""
        self.brief: str = self.help.splitlines()[0] if self.help else ""
        self.cog: Optional[Any] = None
        self.checks: list[Callable] = []
        # Copy any checks attached to the callback directly
        if hasattr(callback, "__command_checks__"):
            self.checks.extend(callback.__command_checks__)

    # ------------------------------------------------------------------
    # Invocation
    # ------------------------------------------------------------------

    async def invoke(self, ctx: Any) -> None:
        """Run the command with *ctx*.

        Performs:
        1. Check evaluation (raises :exc:`CheckFailure` on failure).
        2. Argument parsing from ``ctx.args``.
        3. Calling the callback with converted arguments.

        Parameters
        ----------
        ctx:
            :class:`~parley.ext.commands.context.Context` instance.
        """
        for predicate in self.checks:
            result = predicate(ctx)
            if asyncio.iscoroutine(result):
                result = await result
            if not result:
                raise CheckFailure(f"Check failed for command '{self.name}'")

        args, kwargs = await self._parse_arguments(ctx)
        if self.cog is not None:
            await self.callback(self.cog, ctx, *args, **kwargs)
        else:
            await self.callback(ctx, *args, **kwargs)

    async def _parse_arguments(self, ctx: Any) -> tuple[list, dict]:
        """Parse ``ctx.args`` against the callback signature.

        - Required parameters raise :exc:`MissingRequiredArgument`.
        - The last parameter annotated or named ``*args`` consumes all
          remaining tokens as a joined string.
        """
        sig = inspect.signature(self.callback)
        params = list(sig.parameters.values())

        # Skip 'self' (cog) and 'ctx'
        skip = 1 if self.cog is not None else 0
        skip += 1  # skip ctx
        params = params[skip:]

        tokens: list[str] = list(ctx.args)
        positional: list[Any] = []

        for i, param in enumerate(params):
            is_var_positional = param.kind == inspect.Parameter.VAR_POSITIONAL
            is_keyword_only = param.kind == inspect.Parameter.KEYWORD_ONLY

            if is_var_positional:
                # Consume all remaining tokens
                positional.append(" ".join(tokens))
                tokens = []
                break

            if not tokens:
                if param.default is inspect.Parameter.empty:
                    raise MissingRequiredArgument(param.name)
                # Has a default — skip
                continue

            if i == len(params) - 1:
                # Last positional — consume remainder joined
                value = " ".join(tokens)
                tokens = []
            else:
                value = tokens.pop(0)

            # Type conversion
            ann = param.annotation
            if ann is not inspect.Parameter.empty and ann is not str:
                try:
                    value = ann(value)
                except (ValueError, TypeError) as exc:
                    raise BadArgument(
                        f"Failed to convert '{value}' to {ann.__name__} "
                        f"for parameter '{param.name}'"
                    ) from exc
            positional.append(value)

        return positional, {}

    def all_names(self) -> list[str]:
        """Return the command name and all aliases."""
        return [self.name] + self.aliases

    def __repr__(self) -> str:
        return f"<Command name={self.name!r}>"


class Group(Command):
    """
    A command that contains sub-commands.

    Sub-commands are registered via :meth:`command` and :meth:`group`
    decorators applied on the group object.

    Example::

        @bot.group()
        async def admin(ctx):
            if ctx.invoked_subcommand is None:
                await ctx.send("Use a subcommand: kick, ban")

        @admin.command()
        async def kick(ctx, user: str):
            ...
    """

    def __init__(self, *args: Any, **kwargs: Any) -> None:
        super().__init__(*args, **kwargs)
        self._commands: dict[str, Command] = {}

    def add_command(self, cmd: Command) -> None:
        """Register a sub-command on this group."""
        for name in cmd.all_names():
            self._commands[name] = cmd

    def remove_command(self, name: str) -> Optional[Command]:
        """Remove a sub-command by name."""
        cmd = self._commands.pop(name, None)
        if cmd:
            for alias in cmd.aliases:
                self._commands.pop(alias, None)
        return cmd

    def get_command(self, name: str) -> Optional[Command]:
        """Look up a sub-command by name or alias."""
        return self._commands.get(name)

    def command(
        self,
        name: Optional[str] = None,
        *,
        aliases: Optional[list[str]] = None,
        help: Optional[str] = None,
    ) -> Callable[[T], Command]:
        """Decorator: register a sub-command on this group."""

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
    ) -> Callable[[T], "Group"]:
        """Decorator: register a sub-group on this group."""

        def decorator(fn: T) -> "Group":
            grp = Group(fn, name=name, aliases=aliases, help=help)
            self.add_command(grp)
            return grp

        return decorator  # type: ignore[return-value]

    async def invoke(self, ctx: Any) -> None:
        """Dispatch to a sub-command if one is provided, else run the group callback."""
        subcommand_name = ctx.args[0] if ctx.args else None
        sub = self.get_command(subcommand_name) if subcommand_name else None
        if sub is not None:
            # Peel off the sub-command token from args
            ctx.args = ctx.args[1:]
            ctx.invoked_with = subcommand_name
            if sub.cog is None:
                sub.cog = self.cog
            await sub.invoke(ctx)
        else:
            await super().invoke(ctx)

    def __repr__(self) -> str:
        return f"<Group name={self.name!r} commands={list(self._commands)!r}>"


# ---------------------------------------------------------------------------
# Decorators
# ---------------------------------------------------------------------------


def command(
    name: Optional[str] = None,
    *,
    aliases: Optional[list[str]] = None,
    help: Optional[str] = None,
) -> Callable[[T], Command]:
    """Decorator that turns an async function into a :class:`Command`.

    Can be used standalone or on a :class:`~parley.ext.commands.bot.CommandBot`::

        @bot.command()
        async def ping(ctx):
            await ctx.reply("pong!")

        # Or standalone (add to bot manually):
        @parley.command()
        async def ping(ctx): ...
        bot.add_command(ping)

    Parameters
    ----------
    name:
        Override the command name (defaults to function name).
    aliases:
        Additional names for this command.
    help:
        Help text (defaults to the function's docstring).
    """

    def decorator(fn: T) -> Command:
        return Command(fn, name=name, aliases=aliases, help=help)

    return decorator  # type: ignore[return-value]


def group(
    name: Optional[str] = None,
    *,
    aliases: Optional[list[str]] = None,
    help: Optional[str] = None,
) -> Callable[[T], Group]:
    """Decorator that turns an async function into a :class:`Group`."""

    def decorator(fn: T) -> Group:
        return Group(fn, name=name, aliases=aliases, help=help)

    return decorator  # type: ignore[return-value]


def check(predicate: Callable) -> Callable[[T], T]:
    """Decorator that adds a check predicate to a command.

    The predicate receives the :class:`~parley.ext.commands.context.Context`
    and must return a truthy value (or a coroutine that does so) to allow
    the command to proceed.

    Example::

        def is_admin(ctx):
            return ctx.author.id == ADMIN_ID

        @bot.command()
        @parley.check(is_admin)
        async def secret(ctx):
            await ctx.send("Only for admins!")
    """

    def decorator(fn: T) -> T:
        if not hasattr(fn, "__command_checks__"):
            fn.__command_checks__ = []  # type: ignore[attr-defined]
        fn.__command_checks__.append(predicate)  # type: ignore[attr-defined]
        return fn

    return decorator
