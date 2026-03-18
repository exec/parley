"""
parley.ext.tasks
================
Simple scheduled task loop decorator.

Usage::

    import parley
    from parley.ext import tasks

    @tasks.loop(seconds=30)
    async def status_check():
        print("Checking status…")

    @bot.event
    async def on_ready(payload):
        status_check.start()

    # Or pass the bot for error handling:
    @tasks.loop(minutes=5)
    async def reminder(bot):
        channel = bot.channels[CHANNEL_ID]
        await channel.send("Reminder!")

    reminder.start(bot)
"""

from __future__ import annotations

import asyncio
import logging
from typing import Any, Callable, Coroutine, Optional, TypeVar

__all__ = ["loop", "Loop"]

log = logging.getLogger("parley.tasks")

_CoroFunc = Callable[..., Coroutine[Any, Any, Any]]
T = TypeVar("T", bound=_CoroFunc)


class Loop:
    """
    Represents a coroutine that runs on a fixed time interval.

    Created by the :func:`loop` decorator.  Do not instantiate directly.

    Attributes
    ----------
    coro:
        The underlying coroutine function.
    seconds:
        Total interval in seconds between iterations.
    count:
        Number of times to run (``None`` = infinite).
    current_loop:
        How many times the loop has fired since :meth:`start`.
    """

    def __init__(
        self,
        coro: _CoroFunc,
        *,
        seconds: float = 0.0,
        minutes: float = 0.0,
        hours: float = 0.0,
        count: Optional[int] = None,
        reconnect: bool = True,
    ) -> None:
        self.coro = coro
        self.seconds = seconds + minutes * 60 + hours * 3600
        if self.seconds <= 0:
            raise ValueError(
                "Loop interval must be positive. "
                "Provide at least one of: seconds, minutes, hours."
            )
        self.count = count
        self.reconnect = reconnect
        self.current_loop: int = 0
        self._task: Optional[asyncio.Task] = None
        self._args: tuple = ()
        self._kwargs: dict = {}
        self._before_loop: Optional[_CoroFunc] = None
        self._after_loop: Optional[_CoroFunc] = None
        self._error_handler: Optional[_CoroFunc] = None
        self._stop_next: bool = False

    # ------------------------------------------------------------------
    # Control
    # ------------------------------------------------------------------

    def start(self, *args: Any, **kwargs: Any) -> asyncio.Task:
        """Start the loop.

        Parameters
        ----------
        *args, **kwargs:
            Forwarded to the coroutine on every iteration.

        Returns
        -------
        :class:`asyncio.Task`
            The running background task.
        """
        if self._task is not None and not self._task.done():
            raise RuntimeError(
                f"Loop '{self.coro.__name__}' is already running. "
                "Call stop() before starting again."
            )
        self._args = args
        self._kwargs = kwargs
        self._stop_next = False
        self.current_loop = 0
        self._task = asyncio.ensure_future(self._run())
        return self._task

    def stop(self) -> None:
        """Request the loop to stop after the current iteration completes."""
        self._stop_next = True

    def cancel(self) -> None:
        """Immediately cancel the loop task."""
        if self._task is not None and not self._task.done():
            self._task.cancel()

    def restart(self, *args: Any, **kwargs: Any) -> asyncio.Task:
        """Cancel the current task and restart the loop."""
        self.cancel()
        return self.start(*args or self._args, **kwargs or self._kwargs)

    @property
    def is_running(self) -> bool:
        """``True`` if the loop task is active."""
        return self._task is not None and not self._task.done()

    # ------------------------------------------------------------------
    # Hooks
    # ------------------------------------------------------------------

    def before_loop(self, coro: _CoroFunc) -> _CoroFunc:
        """Decorator: register a coroutine to run once before the loop starts."""
        self._before_loop = coro
        return coro

    def after_loop(self, coro: _CoroFunc) -> _CoroFunc:
        """Decorator: register a coroutine to run once after the loop ends."""
        self._after_loop = coro
        return coro

    def error(self, coro: _CoroFunc) -> _CoroFunc:
        """Decorator: register a coroutine to handle exceptions.

        The handler receives the exception as its only argument.
        If no error handler is registered, exceptions are logged and the
        loop continues (unless *reconnect* is ``False``).
        """
        self._error_handler = coro
        return coro

    # ------------------------------------------------------------------
    # Internal
    # ------------------------------------------------------------------

    async def _run(self) -> None:
        if self._before_loop is not None:
            try:
                await self._before_loop(*self._args, **self._kwargs)
            except Exception:
                log.exception(
                    "Exception in before_loop for task '%s'", self.coro.__name__
                )

        try:
            while True:
                if self._stop_next:
                    break

                if self.count is not None and self.current_loop >= self.count:
                    break

                try:
                    await self.coro(*self._args, **self._kwargs)
                except asyncio.CancelledError:
                    raise
                except Exception as exc:
                    if self._error_handler is not None:
                        try:
                            await self._error_handler(exc)
                        except Exception:
                            log.exception(
                                "Exception in error handler for task '%s'",
                                self.coro.__name__,
                            )
                    else:
                        log.exception(
                            "Unhandled exception in task '%s'", self.coro.__name__
                        )
                    if not self.reconnect:
                        break

                self.current_loop += 1

                if self._stop_next:
                    break
                if self.count is not None and self.current_loop >= self.count:
                    break

                await asyncio.sleep(self.seconds)
        except asyncio.CancelledError:
            pass
        finally:
            if self._after_loop is not None:
                try:
                    await self._after_loop(*self._args, **self._kwargs)
                except Exception:
                    log.exception(
                        "Exception in after_loop for task '%s'", self.coro.__name__
                    )

    def __call__(self, *args: Any, **kwargs: Any) -> Coroutine:
        """Allow using the loop as a regular coroutine for one-off calls."""
        return self.coro(*args, **kwargs)


def loop(
    *,
    seconds: float = 0.0,
    minutes: float = 0.0,
    hours: float = 0.0,
    count: Optional[int] = None,
    reconnect: bool = True,
) -> Callable[[T], Loop]:
    """Decorator that schedules a coroutine to run on a repeating interval.

    Parameters
    ----------
    seconds:
        Seconds between each run.
    minutes:
        Minutes between each run (added to *seconds*).
    hours:
        Hours between each run (added to *seconds* and *minutes*).
    count:
        Maximum number of iterations (``None`` = infinite).
    reconnect:
        Whether to continue the loop after an unhandled exception
        (default ``True``).

    Example::

        @tasks.loop(minutes=1)
        async def heartbeat():
            await channel.send("Still alive!")

        # Start it when the bot is ready
        @bot.event
        async def on_ready(payload):
            heartbeat.start()

    Notes
    -----
    The first iteration runs immediately when :meth:`~Loop.start` is called.
    Subsequent iterations are delayed by the configured interval.
    """

    def decorator(coro: T) -> Loop:
        if not asyncio.iscoroutinefunction(coro):
            raise TypeError(
                f"@tasks.loop: {coro.__name__!r} must be a coroutine function"
            )
        return Loop(
            coro,
            seconds=seconds,
            minutes=minutes,
            hours=hours,
            count=count,
            reconnect=reconnect,
        )

    return decorator  # type: ignore[return-value]
