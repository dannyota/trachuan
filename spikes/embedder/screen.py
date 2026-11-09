"""Throwaway spike: build the screening corpus spike.screen_chunk.

Union of (1) every chunk of every document an expected golden citation names,
including its alt_doc_numbers,
(2) the top 50 chunks per golden query from the two fully embedded models (qwen3
stored vectors, vi_v2) as hard negatives, (3) a seeded random sample of 15,000
chunks. Models are compared on this subset first; finalists run on the full corpus.
"""

import json
import os

import psycopg

from embedders import TorchModel
from models import BY_KEY
from parity import GOLDEN
from recall import _same_doc, dense_rank

FULL_TABLES = {"qwen3": "gold.chunk_embedding", "vi_v2": "spike.chunk_vector_vi_v2"}


def main():
    cases = json.loads(GOLDEN.read_text())
    expected_numbers = {n for c in cases for ec in c["expected_citations"]
                        for n in [ec["doc_number"], *ec.get("alt_doc_numbers", [])]}
    conn = psycopg.connect(os.environ["DATABASE_URL"], autocommit=True)
    docs = conn.execute("select id, coalesce(doc_number, '') from silver.document").fetchall()
    doc_ids = [d for d, num in docs if num and any(_same_doc(num, e) for e in expected_numbers)]
    chunk_ids = {r[0] for r in conn.execute("select id from gold.chunk where document_id = any(%s)", (doc_ids,))}
    print("expected-document chunks", len(chunk_ids), "documents", len(doc_ids))
    for key, table in FULL_TABLES.items():
        spec = BY_KEY[key]
        model = TorchModel(spec.model_id, max_len=512, revision=spec.revision, remote_code=spec.remote_code)
        qv = dict(zip([c["id"] for c in cases], model.encode([c["question"] for c in cases], prompt=spec.query_prompt)))
        before = len(chunk_ids)
        for hits in dense_rank(conn, qv, table, 50).values():
            chunk_ids.update(h.chunk_id for h in hits)
        print(f"hard negatives from {key}: +{len(chunk_ids) - before}")
    conn.execute("select setseed(0.42)")
    sample = {r[0] for r in conn.execute("select id from gold.chunk order by random() limit 15000")}
    before = len(chunk_ids)
    chunk_ids |= sample
    print(f"random sample: +{len(chunk_ids) - before}")
    conn.execute("drop table if exists spike.screen_chunk")
    conn.execute("create table spike.screen_chunk (chunk_id bigint primary key)")
    with conn.cursor() as cur, cur.copy("copy spike.screen_chunk from stdin") as cp:
        for cid in sorted(chunk_ids):
            cp.write_row((cid,))
    print("screen_chunk rows", conn.execute("select count(*) from spike.screen_chunk").fetchone()[0])


if __name__ == "__main__":
    main()
