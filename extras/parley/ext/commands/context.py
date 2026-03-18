"""
parley.ext.commands.context
===========================
:class:`Context` — the invocation context passed to every command callback.

Provides convenient helpers so command authors never need to touch the raw
HTTP client directly.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any, Optional

if TYPE_CHECKING:
    from parley.client import Bot
    from parley.models.channel import Channel
    from parley.models.member import Member
    from parley.models.message import Message
    from parley.models.server import Server
    from parley.models.user import User
    from .core import Command

__all__ = ["Context"]


class Context:
    """
    Represents the context in which a command was invoked.

    Attributes
    ----------
    message:
        The :class:`~parley.models.message.Message` that triggered the command.
    bot:
        The :class:`~parley.client.Bot` (or :class:`CommandBot`) instance.
    command:
        The :class:`~parley.ext.commands.core.Command` that is being invoked.
        May be ``None`` if the command was not found.
    args:
        Positional arguments parsed from the message (after the command name).
    kwargs:
        Keyword arguments (currently always empty; reserved for future use).
    prefix:
        The prefix that was used to invoke this command.
    invoked_with:
        The command name (or alias) that was used.
    """

    __slots__ = (
        "message",
        "bot",
        "command",
        "args",
        "kwargs",
        "prefix",
        "invoked_with",
    )

    def __init__(
        self,
        *,
        message: "Message",
        bot: "Bot",
        command: Optional["Command"] = None,
        args: list[str] | None = None,
        kwargs: dict[str, Any] | None = None,
        prefix: str = "",
        invoked_with: str = "",
    ) -> None:
        self.message = message
        self.bot = bot
        self.command = command
        self.args: list[str] = args or []
        self.kwargs: dict[str, Any] = kwargs or {}
        self.prefix = prefix
        self.invoked_with = invoked_with

    # ------------------------------------------------------------------
    # Convenience properties
    # ------------------------------------------------------------------

    @property
    def author(self) -> Any:
        """The member/user who sent the message.

        Returns the :class:`~parley.models.member.Member` from the state
        cache if available; otherwise a lightweight proxy with the author
        fields from the message.
        """
        state = self.bot._state
        member = state.get_member(self.channel_id_int, self.message.author_id)
        return member or _MessageAuthorProxy(self.message)

    @property
    def channel(self) -> Optional["Channel"]:
        """The :class:`~parley.models.channel.Channel` where the command was sent."""
        return self.bot._state.get_channel(self.message.channel_id)

    @property
    def server(self) -> Optional["Server"]:
        """The :class:`~parley.models.server.Server` the command was invoked in."""
        ch = self.channel
        if ch is not None:
            return self.bot._state.get_server(ch.server_id)
        return None

    @property
    def channel_id(self) -> int:
        """Integer ID of the channel."""
        return self.message.channel_id

    # Internal alias used by author property
    @property
    def channel_id_int(self) -> int:
        ch = self.channel
        return ch.server_id if ch else 0

    # ------------------------------------------------------------------
    # Response helpers
    # ------------------------------------------------------------------

    async def send(self, content: str) -> "Message":
        """Send a message to the same channel as the triggering message.

        Parameters
        ----------
        content:
            Text to send.

        Returns
        -------
        :class:`~parley.models.message.Message`
        """
        return await self.bot.send_message(self.message.channel_id, content)

    async def reply(self, content: str) -> "Message":
        """Send a thread reply to the triggering message.

        Parameters
        ----------
        content:
            Text to send.

        Returns
        -------
        :class:`~parley.models.message.Message`
        """
        return await self.message.reply(content)

    def typing(self):
        """Async context manager that shows a typing indicator in the channel.

        Usage::

            @bot.command()
            async def slow(ctx):
                async with ctx.typing():
                    result = await do_heavy_work()
                await ctx.send(result)
        """
        from ...models.channel import Typing
        return Typing(self.message.channel_id, self._state)

    def __repr__(self) -> str:
        return (
            f"<Context command={self.invoked_with!r} "
            f"author_id={self.message.author_id} "
            f"channel_id={self.message.channel_id}>"
        )


class _MessageAuthorProxy:
    """Lightweight proxy used when no :class:`Member` is cached."""

    __slots__ = ("_msg",)

    def __init__(self, msg: "Message") -> None:
        self._msg = msg

    @property
    def id(self) -> int:
        return self._msg.author_id

    @property
    def username(self) -> str:
        return self._msg.author_username

    @property
    def display_name(self) -> str:
        return self._msg.author_display_name

    @property
    def display(self) -> str:
        return self._msg.author_display

    @property
    def mention(self) -> str:
        return f"<@{self._msg.author_id}>"

    @property
    def is_bot(self) -> bool:
        return self._msg.author_is_bot

    def __repr__(self) -> str:
        return f"<_MessageAuthorProxy id={self.id} username={self.username!r}>"
