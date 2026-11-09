"""Throwaway spike: recall of quantized Vietnamese_Embedding_v2 query models against the stored
full-corpus document vectors (spike.chunk_vector_vi_v2, MPS FP16)."""
import json, os, sys
from pathlib import Path
import psycopg
sys.path.insert(0, str(Path(__file__).parent))
from recall import dense_rank, mrr_at_k, recall_at_k
from v2_onnx import DIR, GOLDEN, Onnx
cases = json.loads(GOLDEN.read_text())
expected = {c["id"]: ([] if c.get("expect_abstain") else c["expected_citations"]) for c in cases}
conn = psycopg.connect(os.environ["DATABASE_URL"])
for f in ("model.onnx", "q8w.onnx", "int8_dynamic.onnx"):
    m = Onnx(str(DIR / f))
    qv = dict(zip(expected, m.encode([c["question"] for c in cases], 256)))
    r = dense_rank(conn, qv, "spike.chunk_vector_vi_v2", 20)
    print(json.dumps({"query_model": f, "recall@8": round(recall_at_k(r, expected, 8), 4), "found@8": round(recall_at_k(r, expected, 8) * 96),
                      "mrr@8": round(mrr_at_k(r, expected, 8), 4), "recall@20": round(recall_at_k(r, expected, 20), 4)}))
