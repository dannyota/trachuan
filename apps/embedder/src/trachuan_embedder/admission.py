"""Bounded admission with ownership held until worker completion."""

import asyncio
from collections.abc import Callable
from typing import TypeVar

T = TypeVar("T")


class Busy(Exception):
    pass


class Deadline(Exception):
    pass


class Admission:
    def __init__(self, active: int, queued: int, wait: float, deadline: float):
        self.slots = asyncio.Semaphore(active)
        self.capacity = active + queued
        self.wait = wait
        self.deadline = deadline
        self.admitted = 0
        self.active = 0

    def _release(self, task: asyncio.Task) -> None:
        self.slots.release()
        self.active -= 1
        if not task.cancelled():
            task.exception()

    async def run(self, work: Callable[[], T]) -> T:
        if self.admitted + self.active >= self.capacity:
            raise Busy
        self.admitted += 1
        acquired = False
        queued = True
        try:
            try:
                await asyncio.wait_for(self.slots.acquire(), timeout=min(self.wait, self.deadline))
            except TimeoutError as exc:
                raise Busy from exc
            acquired = True
            self.active += 1
            task = asyncio.create_task(asyncio.to_thread(work))
            task.add_done_callback(self._release)
            acquired = False
            self.admitted -= 1
            queued = False
            try:
                return await asyncio.wait_for(asyncio.shield(task), timeout=self.deadline)
            except TimeoutError as exc:
                raise Deadline from exc
        finally:
            if acquired:
                self.slots.release()
            if queued:
                self.admitted -= 1
