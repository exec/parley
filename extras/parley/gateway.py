"""
parley.gateway
==============
WebSocket gateway client with automatic reconnection.

The :class:`GatewayClient` owns the WebSocket connection.  It:

1. Obtains a short-lived ticket from the REST API.
2. Connects to ``/ws?ticket=<ticket>``.
3. Delivers raw events to :class:`~parley.state.ConnectionState` for
   parsing and caching before dispatching.
4. Reconnects with exponential back-off on unexpected disconnection.

Protocol details
----------------
- **No heartbeat** or IDENTIFY opcode — just connect and listen.
- To receive channel events send::

    {"type": "CHANNEL_SUBSCRIBE", "payload": {"channel_id": "123"}}

- Server frames arrive as::

    {"type": "EVENT_TYPE", "payload": {...}}
"""

from __future__ import annotations

import asyncio
import json
import logging
from typing import TYPE_CHECKING, Any, Optional

import websockets
import websockets.exceptions

if TYPE_CHECKING:
    from .state import ConnectionState

__all__ = ["GatewayClient"]

log = logging.getLogger("parley.gateway")

_INITIAL_BACKOFF = 1.0
_MAX_BACKOFF = 60.0


class GatewayClient:
    """
    Manages the persistent WebSocket connection to the Parley gateway.

    Parameters
    ----------
    state:
        The :class:`~parley.state.ConnectionState` that will receive parsed
        events and handle caching.
    """

    def __init__(self, state: "ConnectionState") -> None:
        self._state = state
        self._ws: Optional[Any] = None
        self._running = False
        self._ready = asyncio.Event()

    # ------------------------------------------------------------------
    # Public interface
    # ------------------------------------------------------------------

    @property
    def is_connected(self) -> bool:
        """``True`` if the WebSocket is currently open."""
        return self._ws is not None

    async def send(self, msg_type: str, payload: dict) -> None:
        """Send a frame on the active WebSocket connection.

        Parameters
        ----------
        msg_type:
            The ``"type"`` field of the outbound JSON frame.
        payload:
            The ``"payload"`` dict.

        Raises
        ------
        RuntimeError
            If called when the connection is not open.
        """
        if self._ws is None:
            log.warning("GatewayClient.send called with no active connection")
            return
        frame = json.dumps({"type": msg_type, "payload": payload})
        await self._ws.send(frame)

    async def subscribe(self, channel_id: int) -> None:
        """Subscribe to real-time events for *channel_id*.

        Parameters
        ----------
        channel_id:
            Integer channel ID to subscribe to.
        """
        await self.send("CHANNEL_SUBSCRIBE", {"channel_id": str(channel_id)})
        log.debug("Subscribed to channel %d", channel_id)

    async def unsubscribe(self, channel_id: int) -> None:
        """Unsubscribe from events for *channel_id*."""
        await self.send("CHANNEL_UNSUBSCRIBE", {"channel_id": str(channel_id)})
        log.debug("Unsubscribed from channel %d", channel_id)

    async def wait_until_ready(self) -> None:
        """Block until the first successful connection is established."""
        await self._ready.wait()

    def stop(self) -> None:
        """Signal the run loop to exit after the current connection closes."""
        self._running = False

    # ------------------------------------------------------------------
    # Connection loop
    # ------------------------------------------------------------------

    async def run(self) -> None:
        """Connect to the gateway and run the event loop.

        This coroutine runs indefinitely (with auto-reconnect) until
        :meth:`stop` is called or the event loop is cancelled.
        """
        self._running = True
        backoff = _INITIAL_BACKOFF

        while self._running:
            try:
                url = await self._build_ws_url()
                log.info("Connecting to gateway: %s", url.split("?")[0])
                async with websockets.connect(
                    url,
                    ping_interval=None,  # server has no heartbeat
                    close_timeout=5,
                ) as ws:
                    self._ws = ws
                    backoff = _INITIAL_BACKOFF
                    self._ready.set()

                    # Fire internal "connected" hook so state can subscribe channels
                    await self._state._on_gateway_connected()

                    async for raw in ws:
                        await self._handle_frame(raw)

            except websockets.exceptions.ConnectionClosedOK:
                log.info("Gateway connection closed normally.")
                if not self._running:
                    break
                # Reconnect on a normal close too (e.g. server restart)
            except websockets.exceptions.ConnectionClosed as exc:
                log.warning(
                    "Gateway disconnected (%s); reconnecting in %.1fs…", exc, backoff
                )
            except OSError as exc:
                log.warning(
                    "Gateway connection error (%s); reconnecting in %.1fs…",
                    exc,
                    backoff,
                )
            except asyncio.CancelledError:
                log.info("Gateway task cancelled.")
                break
            except Exception as exc:
                log.exception(
                    "Unexpected gateway error; reconnecting in %.1fs…", backoff
                )
            finally:
                self._ws = None

            if self._running:
                await asyncio.sleep(backoff)
                backoff = min(backoff * 2, _MAX_BACKOFF)

    # ------------------------------------------------------------------
    # Internals
    # ------------------------------------------------------------------

    async def _build_ws_url(self) -> str:
        """Obtain a WS ticket and build the full gateway URL."""
        ticket = await self._state.http.get_ws_ticket()
        ws_base = (
            self._state.http.base_url
            .replace("https://", "wss://")
            .replace("http://", "ws://")
        )
        return f"{ws_base}/ws?ticket={ticket}"

    async def _handle_frame(self, raw: str | bytes) -> None:
        """Parse a raw WebSocket frame and forward to state."""
        try:
            data = json.loads(raw)
        except json.JSONDecodeError:
            log.warning("Received non-JSON frame: %r", raw)
            return

        event_type: str = data.get("type", "")
        payload: dict = data.get("payload", {})
        if not isinstance(payload, dict):
            payload = {}

        if not event_type:
            log.debug("Received frame with no type: %r", data)
            return

        log.debug("Gateway event: %s", event_type)
        await self._state._process_event(event_type, payload)
