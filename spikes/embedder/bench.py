"""Throwaway spike: throughput and recipe sanity for each backend on 200 stored chunks."""

import os
import sys
import time
from pathlib import Path

import numpy as np
import psycopg

from embedders import OnnxQwen, TorchModel, doc_text

MODELS = Path("../../work/models/qwen3-0.6b")


def sample(conn, n: int):
    with conn.cursor() as cur:
        cur.execute(
            "select c.id, c.context_prefix, c.content, e.embedding::text from gold.chunk c "
            "join gold.chunk_embedding e on e.chunk_id = c.id order by c.id limit %s offset 40000", (n,))
        rows = cur.fetchall()
    texts = [doc_text(p, c) for _, p, c, _ in rows]
    stored = np.array([np.fromstring(v.strip("[]"), sep=",") for *_, v in rows], dtype=np.float32)
    return texts, stored


def run(name, enc, texts, stored):
    t = time.perf_counter()
    vecs = np.concatenate([enc(texts[i : i + 16]) for i in range(0, len(texts), 16)])
    dt = time.perf_counter() - t
    cos = (vecs * stored).sum(axis=1)
    print(f"{name}: {len(texts)/dt:.1f} chunks/s; cosine vs stored median {np.median(cos):.5f} min {cos.min():.5f}")


if __name__ == "__main__":
    conn = psycopg.connect(os.environ["DATABASE_URL"])
    texts, stored = sample(conn, int(sys.argv[1]) if len(sys.argv) > 1 else 200)
    which = sys.argv[2:] or ["fp32", "int8", "mps"]
    if "fp32" in which:
        run("onnx fp32 cpu", OnnxQwen(MODELS / "model.onnx").encode, texts, stored)
    if "int8" in which:
        run("onnx int8 cpu", OnnxQwen(MODELS / "model_int8.onnx").encode, texts, stored)
    for variant in ("q8w", "q4w", "q8w_a4"):
        if variant in which:
            run(f"onnx {variant} cpu", OnnxQwen(MODELS / f"model_{variant}.onnx").encode, texts, stored)
    if "mps" in which:
        run("torch fp16 mps", TorchModel("Qwen/Qwen3-Embedding-0.6B").encode, texts, stored)
