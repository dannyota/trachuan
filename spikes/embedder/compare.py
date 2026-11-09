"""Throwaway spike: compare dense retrieval of every fully embedded candidate model on banhmi's
Vietnam golden set (87 cases, 96 expected citations): recall@8, MRR@8, recall@20, and per-case
differences against the Qwen3-Embedding-0.6B baseline."""

import json
import os
import sys

import psycopg

from embedders import TorchModel
from models import SPECS
from parity import GOLDEN, OUT
from recall import dense_rank, matches, mrr_at_k, recall_at_k

CHUNKS = 142086
FULL = {"qwen3": "gold.chunk_embedding"}


def table_for(key: str, screen: bool, conn) -> tuple[str | None, str | None]:
    """Return (vector table, subset table) for a model, preferring a full table."""
    full = FULL.get(key, f"spike.chunk_vector_{key}")
    if _rows(conn, full) == CHUNKS:
        return full, ("spike.screen_chunk" if screen else None)
    if screen and _rows(conn, f"spike.screen_vector_{key}") == _rows(conn, "spike.screen_chunk"):
        return f"spike.screen_vector_{key}", None
    return None, None


def _rows(conn, table: str) -> int:
    if not conn.execute("select to_regclass(%s)", (table,)).fetchone()[0]:
        return -1
    return conn.execute(f"select count(*) from {table}").fetchone()[0]


def found_at_8(ranked, cits):
    return sum(any(matches(ec, h) for h in ranked[:8]) for ec in cits)


def main(keys, screen):
    cases = json.loads(GOLDEN.read_text())
    expected = {c["id"]: ([] if c.get("expect_abstain") else c["expected_citations"]) for c in cases}
    conn = psycopg.connect(os.environ["DATABASE_URL"])
    results, per_case = {}, {}
    for spec in SPECS:
        table, subset = table_for(spec.key, screen, conn)
        if (keys and spec.key not in keys) or not table:
            continue
        model = TorchModel(spec.model_id, max_len=512, revision=spec.revision, remote_code=spec.remote_code)
        qv = dict(zip(expected, model.encode([c["question"] for c in cases], prompt=spec.query_prompt)))
        ranked = dense_rank(conn, qv, table, 20, subset)
        results[spec.key] = {"model": spec.model_id, "recall@8": round(recall_at_k(ranked, expected, 8), 4),
                             "mrr@8": round(mrr_at_k(ranked, expected, 8), 4),
                             "recall@20": round(recall_at_k(ranked, expected, 20), 4), "note": spec.note}
        per_case[spec.key] = {cid: found_at_8(ranked[cid], cits) for cid, cits in expected.items() if cits}
        print(json.dumps({spec.key: results[spec.key]}), flush=True)
    base = per_case.get("qwen3", {})
    for key, cases_found in per_case.items():
        if key != "qwen3" and base:
            results[key]["cases_better_than_qwen3"] = sorted(c for c, n in cases_found.items() if n > base[c])
            results[key]["cases_worse_than_qwen3"] = sorted(c for c, n in cases_found.items() if n < base[c])
    (OUT / ("compare-screen.json" if screen else "compare.json")).write_text(json.dumps(results, indent=2, ensure_ascii=False))


if __name__ == "__main__":
    args = sys.argv[1:]
    main([a for a in args if a != "--screen"], "--screen" in args)
