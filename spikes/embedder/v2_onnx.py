"""Throwaway spike: Vietnamese_Embedding_v2 through ONNX Runtime on CPU, FP32 vs quantized.

Prints parity against PyTorch FP32 (200 chunks, 92 golden questions) and, per
variant, process RSS and one-question latency with 4 threads.
"""

import json
import os
import resource
import sys
import time
from pathlib import Path

import numpy as np

MODEL = "AITeamVN/Vietnamese_Embedding_v2"
REV = "18b44161e041"
DIR = Path("../../work/models/vi_v2")
GOLDEN = Path("/Users/danny/src/dannyota/banhmi/deploy/eval/golden.json")


def prepare():
    from huggingface_hub import hf_hub_download
    from onnxruntime.quantization import QuantType, quantize_dynamic
    from onnxruntime.quantization.matmul_nbits_quantizer import MatMulNBitsQuantizer
    import onnx
    DIR.mkdir(parents=True, exist_ok=True)
    import shutil
    for f in ("model.onnx", "model.onnx_data"):
        if not (DIR / f).exists():
            shutil.copyfile(Path(hf_hub_download(MODEL, f"onnx/{f}", revision=REV)).resolve(), DIR / f)
    src = str(DIR / "model.onnx")
    if not (DIR / "int8_dynamic.onnx").exists():
        quantize_dynamic(src, str(DIR / "int8_dynamic.onnx"), weight_type=QuantType.QInt8, per_channel=True,
                         use_external_data_format=True)
    if not (DIR / "q8w.onnx").exists():
        q = MatMulNBitsQuantizer(onnx.load(src, load_external_data=True), bits=8, block_size=128, is_symmetric=True)
        q.process()
        onnx.save_model(q.model.model, str(DIR / "q8w.onnx"), save_as_external_data=True, location="q8w.onnx.data")
    print(json.dumps({"fp32": src}))


class Onnx:
    def __init__(self, path):
        import onnxruntime as ort
        from transformers import AutoTokenizer
        o = ort.SessionOptions()
        o.intra_op_num_threads = 4
        self.s = ort.InferenceSession(path, o, providers=["CPUExecutionProvider"])
        self.tok = AutoTokenizer.from_pretrained(MODEL, revision=REV)
        self.out = self.s.get_outputs()[0].name

    def encode(self, texts, max_len):
        b = self.tok(texts, padding=True, truncation=True, max_length=max_len, return_tensors="np")
        feeds = {i.name: b[i.name].astype(np.int64) for i in self.s.get_inputs() if i.name in b}
        h = self.s.run([self.out], feeds)[0]
        v = h[:, 0] if h.ndim == 3 else h
        return v / np.linalg.norm(v, axis=1, keepdims=True)


def probe(path):
    """Run in a fresh process: load, 92 questions one at a time, report RSS and latency."""
    t = time.perf_counter()
    m = Onnx(path)
    load = time.perf_counter() - t
    qs = [c["question"] for c in json.loads(GOLDEN.read_text())]
    m.encode(qs[:3], 256)
    lat = []
    for q in qs:
        t = time.perf_counter(); m.encode([q], 256); lat.append((time.perf_counter() - t) * 1000)
    peak = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss / 2**30
    print(json.dumps({"model": Path(path).name, "load_s": round(load, 1), "peak_rss_GB": round(peak, 2),
                      "p50_ms": round(float(np.percentile(lat, 50)), 1), "p95_ms": round(float(np.percentile(lat, 95)), 1)}))


def parity(fp32_path):
    import psycopg
    sys.path.insert(0, str(Path(__file__).parent))
    from embedders import TorchModel, doc_text
    conn = psycopg.connect(os.environ["DATABASE_URL"])
    rows = conn.execute("select context_prefix, content from gold.chunk order by md5(id::text) limit 200").fetchall()
    docs = [doc_text(p, c) for p, c in rows]
    qs = [c["question"] for c in json.loads(GOLDEN.read_text())]
    ref = TorchModel(MODEL, device="cpu", fp16=False, max_len=2048, revision=REV)
    ref_d, ref_q = ref.encode(docs, batch_size=8), ref.encode(qs, batch_size=8)
    for name, path in [("fp32_onnx", fp32_path), ("int8_dynamic", str(DIR / "int8_dynamic.onnx")), ("q8w", str(DIR / "q8w.onnx"))]:
        m = Onnx(path)
        d = np.concatenate([m.encode(docs[i:i + 8], 2048) for i in range(0, len(docs), 8)])
        q = np.concatenate([m.encode(qs[i:i + 8], 256) for i in range(0, len(qs), 8)])
        cd, cq = (d * ref_d).sum(1), (q * ref_q).sum(1)
        print(json.dumps({"variant": name, "chunks_median": round(float(np.median(cd)), 5), "chunks_min": round(float(cd.min()), 5),
                          "queries_median": round(float(np.median(cq)), 5), "queries_min": round(float(cq.min()), 5)}))


if __name__ == "__main__":
    {"prepare": lambda: prepare(), "probe": lambda: probe(sys.argv[2]), "parity": lambda: parity(sys.argv[2])}[sys.argv[1]]()
