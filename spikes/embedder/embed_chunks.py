"""Throwaway spike: embed every gold.chunk with one registered model on MPS.

Writes spike.chunk_vector_<key>(chunk_id, embedding vector(dim)); dim comes from the
model's output; resumable; records truncated chunks (longer than the spec's max_len
tokens, prompt included) and timing in work/results/chunk_vector_<key>.json.
"""

import argparse
import io
import json
import os
import time
from pathlib import Path

import psycopg

from embedders import TorchModel, doc_text
from models import BY_KEY

OUT = Path("../../work/results")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("key", choices=sorted(BY_KEY))
    ap.add_argument("--batch", type=int, default=64)
    ap.add_argument("--page", type=int, default=4000)
    ap.add_argument("--device", default="mps", choices=["mps", "cpu"])
    ap.add_argument("--screen", action="store_true", help="embed only spike.screen_chunk into spike.screen_vector_<key>")
    args = ap.parse_args()
    spec = BY_KEY[args.key]
    table = f"spike.{'screen' if args.screen else 'chunk'}_vector_{spec.key}"
    source = "gold.chunk c join spike.screen_chunk s on s.chunk_id = c.id" if args.screen else "gold.chunk c"
    conn = psycopg.connect(os.environ["DATABASE_URL"], autocommit=True)
    conn.execute("create schema if not exists spike")
    model = TorchModel(spec.model_id, device=args.device, max_len=spec.max_len, revision=spec.revision,
                       remote_code=spec.remote_code)
    truncated, done, started, created = 0, 0, time.perf_counter(), False
    while True:
        if created:
            rows = conn.execute(
                f"select c.id, c.context_prefix, c.content from {source} where not exists "
                f"(select 1 from {table} t where t.chunk_id = c.id) order by length(c.content), c.id limit %s", (args.page,)).fetchall()
        else:
            rows = conn.execute(f"select c.id, c.context_prefix, c.content from {source} order by length(c.content), c.id limit %s", (args.page,)).fetchall()
        if not rows:
            break
        texts = [doc_text(p, c) for _, p, c in rows]
        truncated += sum(n > spec.max_len for n in model.token_lengths(texts, spec.doc_prompt))
        vecs = model.encode(texts, batch_size=args.batch, prompt=spec.doc_prompt)
        if not created:
            conn.execute(f"create table if not exists {table} (chunk_id bigint primary key, embedding vector({vecs.shape[1]}) not null)")
            conn.execute(f"delete from {table} where chunk_id = any(%s)", ([r[0] for r in rows],))
            created = True
        buf = io.StringIO()
        for (cid, _, _), v in zip(rows, vecs):
            buf.write(f"{cid}\t[{','.join(f'{x:.6f}' for x in v)}]\n")
        with conn.cursor() as cur, cur.copy(f"copy {table} (chunk_id, embedding) from stdin") as cp:
            cp.write(buf.getvalue())
        done += len(rows)
        print(f"{table}: {done} chunks, {done / (time.perf_counter() - started):.1f}/s, truncated {truncated}", flush=True)
    total = conn.execute(f"select count(*) from {table}").fetchone()[0]
    OUT.mkdir(parents=True, exist_ok=True)
    meta = {"key": spec.key, "model": spec.model_id, "revision": spec.revision, "table": table, "rows": total,
            "max_len": spec.max_len, "truncated_this_run": truncated, "seconds_this_run": round(time.perf_counter() - started)}
    (OUT / f"{table.split('.')[1]}.json").write_text(json.dumps(meta, indent=2))
    print(json.dumps(meta))


if __name__ == "__main__":
    main()
