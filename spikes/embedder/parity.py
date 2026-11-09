"""Throwaway spike: FP32 vs quantized parity on 1,000 chunk probes and the golden queries,
then dense recall@8 and MRR@8 for each query model against each document-vector table."""

import json
import os
import sys
from pathlib import Path

import numpy as np
import psycopg

from embedders import QWEN_QUERY_PREFIX, OnnxQwen, doc_text
from recall import dense_rank, mrr_at_k, recall_at_k

MODELS = Path("../../work/models/qwen3-0.6b")
GOLDEN = Path("/Users/danny/src/dannyota/banhmi/deploy/eval/golden.json")
OUT = Path("../../work/results")
VARIANTS = {"fp32": "model.onnx", "int8_dynamic": "model_int8.onnx", "q8w": "model_q8w.onnx", "q8w_a4": "model_q8w_a4.onnx"}


def load_golden():
    cases = json.loads(GOLDEN.read_text())
    queries = {c["id"]: QWEN_QUERY_PREFIX + c["question"] for c in cases}
    expected = {c["id"]: ([] if c.get("expect_abstain") else c["expected_citations"]) for c in cases}
    return queries, expected


def probes(conn, n=1000):
    with conn.cursor() as cur:
        cur.execute("select context_prefix, content from gold.chunk order by md5(id::text) limit %s", (n,))
        return [doc_text(p, c) for p, c in cur.fetchall()]


def encode(model, texts, max_len, batch=8):
    order = sorted(range(len(texts)), key=lambda i: len(texts[i]))
    out = np.zeros((len(texts), 1024), dtype=np.float32)
    for s in range(0, len(order), batch):
        idx = order[s : s + batch]
        out[idx] = model.encode([texts[i] for i in idx], max_len=max_len)
    return out


def stats(ref, got):
    cos = (ref * got).sum(axis=1)
    return {"median": float(np.median(cos)), "min": float(cos.min())}


def main(tables):
    OUT.mkdir(parents=True, exist_ok=True)
    conn = psycopg.connect(os.environ["DATABASE_URL"])
    queries, expected = load_golden()
    qids = list(queries)
    chunk_texts = probes(conn)
    vecs = {}
    for name, file in VARIANTS.items():
        model = OnnxQwen(MODELS / file)
        vecs[name] = {"chunks": encode(model, chunk_texts, 8192),
                      "queries": encode(model, [queries[q] for q in qids], 512)}
        print(f"encoded {name}", flush=True)
    result = {"parity": {}, "recall": {}}
    for name in VARIANTS:
        if name == "fp32":
            continue
        result["parity"][name] = {part: stats(vecs["fp32"][part], vecs[name][part]) for part in ("chunks", "queries")}
    for table in tables:
        for name in VARIANTS:
            qv = dict(zip(qids, vecs[name]["queries"]))
            ranked = dense_rank(conn, qv, table, 8)
            result["recall"][f"{table} | {name}"] = {"recall@8": recall_at_k(ranked, expected, 8),
                                                    "mrr@8": mrr_at_k(ranked, expected, 8)}
    np.save(OUT / "golden_queries_fp32.npy", vecs["fp32"]["queries"])
    (OUT / "parity.json").write_text(json.dumps(result, indent=2))
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main(sys.argv[1:] or ["gold.chunk_embedding"])
