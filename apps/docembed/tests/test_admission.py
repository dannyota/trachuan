import asyncio
import threading

import pytest

from trachuan_docembed.admission import Admission, Busy, Deadline


def test_admission_busy_429_when_queue_full():
    async def run():
        gate = Admission(active=1, queued=0, wait=0.05, deadline=0.05)
        started, release = threading.Event(), threading.Event()

        def work():
            started.set()
            release.wait()

        first = asyncio.create_task(gate.run(work))
        while not started.is_set():
            await asyncio.sleep(0)
        with pytest.raises(Busy):
            await gate.run(lambda: None)
        release.set()
        await first

    asyncio.run(run())


def test_cancelled_request_keeps_active_slot_until_inference_ends():
    async def run():
        gate = Admission(active=1, queued=0, wait=0.05, deadline=0.05)
        started, release = threading.Event(), threading.Event()

        def work():
            started.set()
            release.wait()

        first = asyncio.create_task(gate.run(work))
        while not started.is_set():
            await asyncio.sleep(0)
        first.cancel()
        with pytest.raises(asyncio.CancelledError):
            await first
        with pytest.raises(Busy):
            await gate.run(lambda: None)
        release.set()
        await asyncio.sleep(0.01)
        assert await gate.run(lambda: 42) == 42

    asyncio.run(run())


def test_deadline_504():
    async def run():
        gate = Admission(active=1, queued=0, wait=0.01, deadline=0.01)
        release = threading.Event()
        with pytest.raises(Deadline):
            await gate.run(lambda: release.wait())
        release.set()
        await asyncio.sleep(0.01)

    asyncio.run(run())


def test_document_mode_admission_values(monkeypatch):
    from trachuan_docembed import app

    values = []
    monkeypatch.setattr(app, "Admission", lambda *args: values.append(args))
    monkeypatch.setattr(app, "recipe_hash", lambda _: "recipe")
    app.create_app(object(), object(), {})
    assert values == [(1, 1, 30.0, 120.0)]
