"""Throwaway spike: open-loop soak against serve.py; prints latency percentiles and server RSS.

Arrivals are scheduled every 1/rate seconds regardless of completions, so queueing
shows up in the tail. Two input sizes: golden questions as written ("short") and
inputs padded to about 256 tokens.
"""

import argparse
import asyncio
import json
import time
from pathlib import Path

import httpx
import numpy as np

HERE = Path(__file__).parent


def inputs(kind: str) -> list[str]:
    questions = json.loads((HERE / "golden_questions.json").read_text())
    if kind == "short":
        return questions
    return [" ".join([q] * 9) for q in questions]


async def run(url: str, rate: float, seconds: int, kind: str) -> dict:
    texts = inputs(kind)
    latencies, errors = [], 0
    async with httpx.AsyncClient(timeout=30) as client:
        async def one(text):
            nonlocal errors
            t = time.perf_counter()
            try:
                r = await client.post(f"{url}/query/embeddings", json={"input": [text]})
                r.raise_for_status()
                latencies.append(time.perf_counter() - t)
            except httpx.HTTPError:
                errors += 1

        tasks, start, i = [], time.perf_counter(), 0
        while time.perf_counter() - start < seconds:
            tasks.append(asyncio.create_task(one(texts[i % len(texts)])))
            i += 1
            await asyncio.sleep(max(0.0, start + i / rate - time.perf_counter()))
        await asyncio.gather(*tasks)
        server = await _stats(client, url)
    return {"mode": "open", "kind": kind, "rate": rate, "seconds": seconds, "requests": i, "errors": errors,
            **_percentiles(latencies), "server": server}


async def sequential(url: str, n: int, kind: str) -> dict:
    """Service time: one request at a time, no queueing."""
    texts = inputs(kind)
    latencies = []
    async with httpx.AsyncClient(timeout=60) as client:
        for i in range(n):
            t = time.perf_counter()
            r = await client.post(f"{url}/query/embeddings", json={"input": [texts[i % len(texts)]]})
            r.raise_for_status()
            latencies.append(time.perf_counter() - t)
        server = await _stats(client, url)
    return {"mode": "sequential", "kind": kind, "requests": n, **_percentiles(latencies), "server": server}


async def _stats(client, url):
    try:
        return (await client.get(f"{url}/stats", timeout=120)).json()
    except httpx.HTTPError as e:
        return {"error": type(e).__name__}


def _percentiles(latencies):
    if not latencies:
        return {"completed": 0}
    ms = np.array(latencies) * 1000
    return {"completed": len(latencies), **{f"p{q}_ms": round(float(np.percentile(ms, q)), 1) for q in (50, 95, 99)}}


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", default="http://127.0.0.1:8089")
    ap.add_argument("--rate", type=float, default=2.0)
    ap.add_argument("--seconds", type=int, default=600)
    ap.add_argument("--kind", choices=["short", "256"], default="256")
    ap.add_argument("--sequential", type=int, default=0, help="run N one-at-a-time requests instead of the open-loop soak")
    a = ap.parse_args()
    if a.sequential:
        print(json.dumps(asyncio.run(sequential(a.url, a.sequential, a.kind))))
    else:
        print(json.dumps(asyncio.run(run(a.url, a.rate, a.seconds, a.kind))))
