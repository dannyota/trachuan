"""Throwaway spike: recall of Qwen3 query precisions against banhmi's stored FP16 document vectors."""
import json, os, sys
from pathlib import Path
import psycopg
sys.path.insert(0, str(Path(__file__).parent))
from embedders import QWEN_QUERY_PREFIX, OnnxQwen
from parity import GOLDEN, MODELS, VARIANTS
from recall import dense_rank, mrr_at_k, recall_at_k
cases = json.loads(GOLDEN.read_text())
expected = {c["id"]: ([] if c.get("expect_abstain") else c["expected_citations"]) for c in cases}
conn = psycopg.connect(os.environ["DATABASE_URL"])
for name, file in VARIANTS.items():
    m = OnnxQwen(MODELS / file, threads=4)
    qv = dict(zip(expected, m.encode([QWEN_QUERY_PREFIX + c["question"] for c in cases], max_len=512)))
    r = dense_rank(conn, qv, "gold.chunk_embedding", 8)
    print(json.dumps({"query_model": name, "found@8": round(recall_at_k(r, expected, 8) * 96), "mrr@8": round(mrr_at_k(r, expected, 8), 4)}))
