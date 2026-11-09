"""Throwaway spike: minimal query embedder (POST /query/embeddings) for latency and RSS soaks.

MODEL_PATH selects the ONNX graph; tokenizer.json must sit beside it. The Qwen
query instruction is prepended here, queries are capped at 512 tokens.
"""

import os
import resource
import sys
import time

import psutil
from fastapi import FastAPI
from pydantic import BaseModel

from embedders import QWEN_QUERY_PREFIX, OnnxQwen

_t0 = time.perf_counter()
MODEL = OnnxQwen(os.environ["MODEL_PATH"], threads=int(os.environ.get("THREADS", "0")))
MODEL.encode([QWEN_QUERY_PREFIX + "khởi động"], max_len=512)
LOAD_SECONDS = time.perf_counter() - _t0
app = FastAPI()


class Request(BaseModel):
    input: list[str]


@app.post("/query/embeddings")
def embed(req: Request):
    vecs = MODEL.encode([QWEN_QUERY_PREFIX + t for t in req.input], max_len=512)
    return {"data": [{"index": i, "embedding": v.tolist()} for i, v in enumerate(vecs)]}


@app.get("/stats")
async def stats():
    peak = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
    peak_mb = peak / (1024 * 1024) if sys.platform == "darwin" else peak / 1024
    return {"model": os.environ["MODEL_PATH"], "load_seconds": round(LOAD_SECONDS, 2),
            "rss_mb": round(psutil.Process().memory_info().rss / 2**20), "peak_rss_mb": round(peak_mb)}
