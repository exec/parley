"""
parley.client
=============
Top-level client classes.

- :class:`Client`    — base async client (REST + WebSocket).
- :class:`Bot`       — authenticates with an API key (``plk_…``).
- :class:`Selfbot`   — authenticates as a regular user (JWT or API key).
- :class:`CommandBot`— extends :class:`Bot` with the command framework.

Typical usage::

    bot = parley.CommandBot("https://parley.x86-64.com", api_key="plk_...", command_prefix="!")

    @bot.command()
    async def ping(ctx):
        await ctx.reply("pong!")

    bot.run()
"""

from __future__ import annotations

import asyncio
import inspect
import logging
from typing import Any, Callable, Coroutine, Optional, TypeVar

import httpx

from .errors import AuthError
from .gateway import GatewayClient
from .http import HTTPClient
from .slash import InteractionContext, SlashCommand, SlashOption
from .models.channel import Channel, TextChannel, channel_from_data
from .models.dm import DmChannel
from .models.invite import Invite
from .models.member import Member
from .models.message import DmMessage, Message
from .models.role import Role
from .models.server import Server
from .models.user import PublicUser, User
from .state import ConnectionState

__all__ = ["Client", "Bot", "Selfbot", "CommandBot"]

log = logging.getLogger("parley.client")

_CoroFunc = Callable[..., Coroutine[Any, Any, Any]]
T = TypeVar("T", bound=_CoroFunc)


class Client:
    """
    Base async client combining the REST API and the WebSocket gateway.

    Do not instantiate this class directly; use :class:`Bot` or
    :class:`Selfbot` instead.

    Parameters
    ----------
    base_url:
        Root URL of the Parley instance.
    token:
        Bearer token — JWT or ``plk_…`` API key.
    timeout:
        HTTP request timeout in seconds.
    """

    def __init__(
        self,
        base_url: str,
        token: str,
        *,
        timeout: float = 30.0,
    ) -> None:
        self._base_url = base_url.rstrip("/")
        self._token = token
        self._timeout = timeout

        self.http = HTTPClient(self._base_url, self._token, timeout=self._timeout)
        self._state = ConnectionState(self.http, self._raw_dispatch)
        self._gateway = GatewayClient(self._state)
        self._state.gateway = self._gateway
        self._state._client = self

        # event_name (lowercase) → list of async handlers
        self._listeners: dict[str, list[_CoroFunc]] = {}
        self._task: Optional[asyncio.Task] = None

        # Status state — persisted across reconnects
        self._status_type: str = "online"
        self._status_text: str = ""
        self._degraded: bool = False

    # ------------------------------------------------------------------
    # Properties
    # ------------------------------------------------------------------

    @property
    def user(self) -> Optional[User]:
        """The authenticated user's own profile (populated after connect)."""
        return self._state.user

    @property
    def servers(self) -> dict[int, Server]:
        """Read-only view of the cached server mapping (id → :class:`Server`)."""
        return self._state.servers

    @property
    def channels(self) -> dict[int, Channel]:
        """Read-only view of the cached channel mapping (id → :class:`Channel`)."""
        return self._state.channels

    # ------------------------------------------------------------------
    # Event registration
    # ------------------------------------------------------------------

    def event(self, coro: T) -> T:
        """Register a coroutine as an event handler by naming convention.

        The function name must start with ``on_``, e.g.::

            @client.event
            async def on_message_create(message):
                ...

        Parameters
        ----------
        coro:
            An ``async def`` function whose name begins with ``on_``.

        Returns
        -------
        The same function (decorator pattern).
        """
        if not asyncio.iscoroutinefunction(coro):
            raise TypeError(f"Event handler {coro.__name__!r} must be a coroutine function")
        name = coro.__name__
        if name.startswith("on_"):
            event_name = name[3:]
        else:
            event_name = name
        self._listeners.setdefault(event_name, []).append(coro)
        return coro  # type: ignore[return-value]

    def add_listener(self, coro: _CoroFunc, name: Optional[str] = None) -> None:
        """Programmatically add an event listener.

        Parameters
        ----------
        coro:
            Async callable to invoke when *name* fires.
        name:
            Event name (without ``on_`` prefix).  If not provided the
            function's ``__name__`` is used after stripping ``on_``.
        """
        if not asyncio.iscoroutinefunction(coro):
            raise TypeError(f"Listener {coro!r} must be a coroutine function")
        key = (name or getattr(coro, "__name__", "")).lstrip("on_").lower()
        if key.startswith("on_"):
            key = key[3:]
        self._listeners.setdefault(key, []).append(coro)

    def remove_listener(self, coro: _CoroFunc, name: Optional[str] = None) -> None:
        """Remove a previously registered listener."""
        key = (name or getattr(coro, "__name__", "")).lower()
        if key.startswith("on_"):
            key = key[3:]
        handlers = self._listeners.get(key, [])
        try:
            handlers.remove(coro)
        except ValueError:
            pass

    # ------------------------------------------------------------------
    # Dispatch
    # ------------------------------------------------------------------

    def _raw_dispatch(self, event_name: str, *args: Any) -> None:
        """Called by :class:`~parley.state.ConnectionState` to fire events.

        Schedules all registered handlers as asyncio tasks.
        """
        # Check for method override on subclass (on_message_create etc.)
        method_name = f"on_{event_name}"
        method = getattr(type(self), method_name, None)
        if method is not None and method is not getattr(Client, method_name, None):
            # Subclass has overridden the method
            asyncio.ensure_future(self._call_handler(getattr(self, method_name), *args))

        for handler in self._listeners.get(event_name, []):
            asyncio.ensure_future(self._call_handler(handler, *args))

    async def _reapply_status(self) -> None:
        """Re-apply non-default status after reconnect. Called by ConnectionState before on_ready."""
        if self._status_type != "online" or self._status_text:
            try:
                await self.http.set_status(self._status_type, self._status_text)
            except Exception:
                log.warning("Failed to re-apply status on reconnect")

    async def set_status(self, status_type: str, text: str = "") -> None:
        """Set status. status_type: online | idle | dnd | invisible"""
        await self.http.set_status(status_type, text)
        self._status_type = status_type
        self._status_text = text

    async def set_degraded(self, degraded: bool, reason: str = "") -> None:
        """
        Convenience wrapper for signalling bot health to users.
        True  → DND with optional reason text.
        False → online, status text cleared.
        State persists across reconnects.
        """
        self._degraded = degraded
        if degraded:
            await self.set_status("dnd", reason)
        else:
            await self.set_status("online", "")

    async def _call_handler(self, handler: _CoroFunc, *args: Any) -> None:
        try:
            await handler(*args)
        except Exception:
            log.exception("Error in event handler %r", handler)

    def dispatch(self, event_name: str, *args: Any) -> None:
        """Public dispatch method; fires *on_{event_name}* handlers.

        Parameters
        ----------
        event_name:
            Event name without ``on_`` prefix.
        *args:
            Arguments forwarded to each handler.
        """
        self._raw_dispatch(event_name, *args)

    # ------------------------------------------------------------------
    # REST convenience methods
    # ------------------------------------------------------------------

    async def fetch_me(self) -> User:
        """Fetch the authenticated user's profile and update the cache."""
        data = await self.http.get_me()
        self._state.user = User._from_data(data, self._state)
        return self._state.user

    async def edit_profile(self, **fields) -> User:
        """Update own profile. Allowed fields: username, display_name, avatar_url."""
        data = await self.http.edit_me(**fields)
        self._state.user = User._from_data(data, self._state)
        return self._state.user

    async def send_typing(self, channel_id: int, duration: int = 5) -> None:
        """Send a typing indicator to *channel_id* for *duration* seconds (1-60)."""
        await self.http.send_typing(channel_id, duration)

    async def fetch_user(self, user_id: int) -> PublicUser:
        """Fetch a user's public profile by ID."""
        data = await self.http.get_user(user_id)
        return PublicUser._from_data(data, self._state)

    async def search_users(self, query: str) -> list[PublicUser]:
        """Search for users by username."""
        data = await self.http.search_users(query)
        return [PublicUser._from_data(u, self._state) for u in data]

    async def fetch_servers(self) -> list[Server]:
        """Fetch the list of servers the authenticated user belongs to."""
        data = await self.http.get_servers()
        servers = [Server._from_data(s, self._state) for s in data]
        for s in servers:
            self._state.servers[s.id] = s
        return servers

    async def fetch_server(self, server_id: int) -> Server:
        """Fetch a single server by ID."""
        data = await self.http.get_server(server_id)
        server = Server._from_data(data, self._state)
        self._state.servers[server.id] = server
        return server

    async def create_server(self, name: str) -> Server:
        """Create a new server."""
        data = await self.http.create_server(name)
        server = Server._from_data(data, self._state)
        self._state.servers[server.id] = server
        return server

    async def fetch_channel(self, channel_id: int) -> Channel:
        """Fetch a channel by ID."""
        data = await self.http.get_channel(channel_id)
        ch = channel_from_data(data, self._state)
        self._state.channels[ch.id] = ch
        return ch

    async def fetch_messages(
        self,
        channel_id: int,
        *,
        limit: int = 50,
        before: Optional[int] = None,
    ) -> list[Message]:
        """Fetch message history for *channel_id*."""
        data = await self.http.get_messages(channel_id, limit=limit, before=before)
        msgs = [Message._from_data(m, self._state) for m in data]
        for m in msgs:
            self._state._add_message(m)
        return msgs

    async def send_message(
        self,
        channel_id: int,
        content: str,
        *,
        reply_to: Optional[int] = None,
    ) -> Message:
        """Send a message to *channel_id*.

        Parameters
        ----------
        channel_id:
            Target channel.
        content:
            Message text.
        reply_to:
            Optional parent message ID for a thread reply.
        """
        data = await self.http.create_message(channel_id, content, parent_id=reply_to)
        msg = Message._from_data(data, self._state)
        self._state._add_message(msg)
        return msg

    async def edit_message(self, message_id: int, content: str) -> Message:
        """Edit a message by ID."""
        data = await self.http.edit_message(message_id, content)
        msg = Message._from_data(data, self._state)
        cached = self._state._message_map.get(message_id)
        if cached:
            cached._update(data)
        return msg

    async def delete_message(self, message_id: int) -> None:
        """Delete a message by ID."""
        await self.http.delete_message(message_id)
        self._state._message_map.pop(message_id, None)

    async def fetch_dms(self) -> list[DmChannel]:
        """Fetch all DM channels for the authenticated user."""
        data = await self.http.get_dms()
        dms = [DmChannel._from_data(d, self._state) for d in data]
        for dm in dms:
            self._state.dm_channels[dm.id] = dm
        return dms

    async def open_dm(self, user_id: int) -> DmChannel:
        """Open (or retrieve) a DM channel with *user_id*."""
        data = await self.http.open_dm(user_id)
        dm = DmChannel._from_data(data, self._state)
        self._state.dm_channels[dm.id] = dm
        return dm

    async def fetch_invite(self, code: str) -> Invite:
        """Fetch public info about an invite (no auth required)."""
        data = await self.http.get_invite(code)
        return Invite._from_data(data, self._state)

    async def upload_file(self, file_bytes: bytes, filename: str) -> str:
        """Upload a file and return its public URL."""
        return await self.http.upload_file(file_bytes, filename)

    async def subscribe_channel(self, channel_id: int) -> None:
        """Subscribe to real-time events for a channel via the gateway."""
        await self._gateway.subscribe(channel_id)

    async def unsubscribe_channel(self, channel_id: int) -> None:
        """Unsubscribe from real-time events for a channel."""
        await self._gateway.unsubscribe(channel_id)

    async def wait_until_ready(self) -> None:
        """Block until the gateway has connected and the client is ready."""
        await self._gateway.wait_until_ready()

    # ------------------------------------------------------------------
    # Login (shared between Bot/Selfbot factories)
    # ------------------------------------------------------------------

    @classmethod
    async def _do_login(cls, base_url: str, email: str, password: str) -> str:
        """Perform credential login and return the auth token."""
        async with httpx.AsyncClient(
            base_url=base_url, timeout=30.0
        ) as http:
            r = await http.post(
                "/api/auth/login",
                json={"email": email, "password": password},
                headers={"Content-Type": "application/json"},
            )
            if r.status_code >= 400:
                try:
                    msg = r.json().get("error", r.text)
                except Exception:
                    msg = r.text
                raise AuthError(f"Login failed ({r.status_code}): {msg}", r.status_code)
            data = r.json()
            token = data.get("token") or data.get("access_token")
            if not token:
                raise AuthError("Login response did not contain a token")
        return token

    # ------------------------------------------------------------------
    # Run loop
    # ------------------------------------------------------------------

    def run(self) -> None:
        """Start the client and block until it stops.

        This is a synchronous convenience wrapper around :meth:`start`.
        Handles :exc:`KeyboardInterrupt` gracefully.
        """
        try:
            asyncio.run(self.start())
        except KeyboardInterrupt:
            pass

    async def start(self) -> None:
        """Async entry point — connect the gateway and run forever."""
        try:
            await self._gateway.run()
        finally:
            await self.close()

    async def close(self) -> None:
        """Gracefully close the gateway and HTTP connections."""
        self._gateway.stop()
        await self.http.close()

    def stop(self) -> None:
        """Signal the client to stop after the current connection closes."""
        self._gateway.stop()

    # ------------------------------------------------------------------
    # Override points for subclasses
    # ------------------------------------------------------------------

    async def on_ready(self, payload: dict) -> None:
        """Called when the client first connects and is ready.

        Override this in a subclass::

            class MyBot(parley.Bot):
                async def on_ready(self, payload):
                    print(f"Ready as {self.user.display}")
        """

    def __repr__(self) -> str:
        user = self._state.user
        name = user.username if user else "not connected"
        return f"<{type(self).__name__} user={name!r}>"


# ---------------------------------------------------------------------------
# Bot
# ---------------------------------------------------------------------------


class Bot(Client):
    """
    Bot client authenticating with a Parley API key (``plk_…``).

    Parameters
    ----------
    base_url:
        Root URL of the Parley instance.
    api_key:
        Developer API key obtained from ``/developer/keys``.
    timeout:
        HTTP timeout in seconds.
    """

    def __init__(
        self,
        base_url: str,
        *,
        api_key: str,
        timeout: float = 30.0,
    ) -> None:
        if not api_key:
            raise ValueError("api_key is required for Bot")
        if not api_key.startswith("plk_"):
            log.warning(
                "API key %r does not start with 'plk_' — verify this is correct",
                api_key[:8] + "…",
            )
        super().__init__(base_url, api_key, timeout=timeout)

        # Slash-command registry: name → SlashCommand
        self._slash_commands: dict[str, SlashCommand] = {}

        # Hook READY to register commands and INTERACTION_CREATE to dispatch.
        self.add_listener(self._slash_on_ready, "ready")
        self.add_listener(self._slash_on_interaction, "interaction_create")

    # ------------------------------------------------------------------
    # Slash-command registration
    # ------------------------------------------------------------------

    def slash_command(
        self,
        *,
        name: str,
        description: str,
        options: Optional[list] = None,
    ) -> Callable[[_CoroFunc], SlashCommand]:
        """Decorator registering a slash command on this bot.

        Example::

            @bot.slash_command(
                name="weather",
                description="Current weather",
                options=[SlashOption("city", "City name", type="STRING", required=True)],
            )
            async def weather(ctx, city: str):
                await ctx.respond(f"Weather in {city}: sunny")

        The wrapped handler must be an ``async def`` accepting an
        :class:`~parley.slash.InteractionContext` followed by option kwargs.

        Parameters
        ----------
        name:
            Command name (1-32 chars, lowercase).
        description:
            Short description shown to users (1-100 chars).
        options:
            Optional list of :class:`~parley.slash.SlashOption`.
        """

        def decorator(fn: _CoroFunc) -> SlashCommand:
            if not asyncio.iscoroutinefunction(fn):
                raise TypeError(
                    f"slash_command handler {fn.__name__!r} must be a coroutine function"
                )
            cmd = SlashCommand(
                name=name,
                description=description,
                options=list(options or []),
                handler=fn,
            )
            self.add_slash_command(cmd)
            return cmd

        return decorator

    def add_slash_command(self, cmd: SlashCommand) -> None:
        """Register a :class:`~parley.slash.SlashCommand` on this bot."""
        if cmd.name in self._slash_commands:
            log.warning("Overwriting existing slash command %r", cmd.name)
        self._slash_commands[cmd.name] = cmd

    def remove_slash_command(self, name: str) -> Optional[SlashCommand]:
        """Remove a slash command by name."""
        return self._slash_commands.pop(name, None)

    def get_slash_command(self, name: str) -> Optional[SlashCommand]:
        """Retrieve a registered slash command by name."""
        return self._slash_commands.get(name)

    @property
    def slash_commands(self) -> dict[str, SlashCommand]:
        """Read-only view of the registered slash-command mapping."""
        return dict(self._slash_commands)

    # ------------------------------------------------------------------
    # Slash-command lifecycle hooks
    # ------------------------------------------------------------------

    async def _slash_on_ready(self, _payload: dict) -> None:
        """Push the slash-command registry to every server the bot is in."""
        if not self._slash_commands:
            return

        body = [c.to_api() for c in self._slash_commands.values()]
        server_ids = list(self._state.servers.keys())
        if not server_ids:
            log.debug(
                "Bot is in no servers; skipping slash-command registration"
            )
            return

        for server_id in server_ids:
            path = f"/api/bots/@me/servers/{server_id}/commands"
            try:
                await self.http.request("PUT", path, json=body)
                log.info(
                    "Registered %d slash command(s) on server %d",
                    len(body),
                    server_id,
                )
            except Exception:
                log.warning(
                    "Failed to register slash commands on server %d",
                    server_id,
                    exc_info=True,
                )

    async def _slash_on_interaction(self, payload: dict) -> None:
        """Route INTERACTION_CREATE to the registered handler by command name."""
        try:
            cmd_info = payload.get("command") or {}
            cmd_name = str(cmd_info.get("name", ""))
        except Exception:
            log.warning("Malformed INTERACTION_CREATE payload: %r", payload)
            return

        cmd = self._slash_commands.get(cmd_name)
        if cmd is None or cmd.handler is None:
            log.warning(
                "Received interaction for unregistered command %r", cmd_name
            )
            return

        ctx = InteractionContext(self, payload)
        kwargs = dict(ctx.options)

        try:
            await cmd.handler(ctx, **kwargs)
        except Exception:
            log.exception(
                "Slash command %r raised an exception", cmd_name
            )
            # Best-effort error notification. If the handler already
            # responded (or the interaction is gone), silently swallow.
            if not ctx.responded:
                try:
                    await ctx.respond("An error occurred.")
                except Exception:
                    log.debug(
                        "Failed to send fallback error response for %r",
                        cmd_name,
                        exc_info=True,
                    )


# ---------------------------------------------------------------------------
# Selfbot
# ---------------------------------------------------------------------------


class Selfbot(Client):
    """
    Selfbot client authenticating as a regular Parley user.

    Accepts either a JWT session token (from the login API or browser
    localStorage) or a developer API key.  Both are sent as a Bearer token.

    Parameters
    ----------
    base_url:
        Root URL of the Parley instance.
    token:
        JWT session token.
    api_key:
        Developer API key (used if *token* is not provided).
    timeout:
        HTTP timeout in seconds.

    Examples
    --------
    With a JWT token::

        sb = parley.Selfbot("https://parley.x86-64.com", token="eyJ...")

    With an API key::

        sb = parley.Selfbot("https://parley.x86-64.com", api_key="plk_...")

    Via email/password login::

        sb = await parley.Selfbot.login("https://...", email="me@example.com", password="...")
    """

    def __init__(
        self,
        base_url: str,
        *,
        token: str = "",
        api_key: str = "",
        timeout: float = 30.0,
    ) -> None:
        cred = token or api_key
        if not cred:
            raise ValueError("Selfbot requires either token= or api_key=")
        super().__init__(base_url, cred, timeout=timeout)

    @classmethod
    async def login(
        cls,
        base_url: str,
        *,
        email: str,
        password: str,
        timeout: float = 30.0,
    ) -> "Selfbot":
        """Create a :class:`Selfbot` by logging in with email and password.

        Parameters
        ----------
        base_url:
            Root URL of the Parley instance.
        email:
            User's email address.
        password:
            User's password.

        Returns
        -------
        :class:`Selfbot`
            An authenticated selfbot instance.
        """
        token = await Client._do_login(base_url, email, password)
        return cls(base_url, token=token, timeout=timeout)


# ---------------------------------------------------------------------------
# CommandBot  (imported here to avoid circular imports)
# ---------------------------------------------------------------------------

# CommandBot is defined in ext/commands/bot.py and re-imported here.
# We do the import lazily so the ext package can itself reference parley.Bot.

def __getattr__(name: str) -> Any:
    if name == "CommandBot":
        from .ext.commands.bot import CommandBot
        return CommandBot
    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")
