"""Throwaway spike: query latency and RSS for one model on CPU, golden questions one at a time."""
import json, sys, time
import numpy as np, psutil, torch
sys.path.insert(0, str(__import__("pathlib").Path(__file__).parent))
from embedders import TorchModel
from models import BY_KEY
torch.set_num_threads(4)
spec = BY_KEY[sys.argv[1]]
proc = psutil.Process()
base = proc.memory_info().rss
t0 = time.perf_counter()
m = TorchModel(spec.model_id, device="cpu", max_len=512, revision=spec.revision, remote_code=spec.remote_code)
load = time.perf_counter() - t0
qs = [c["question"] for c in json.load(open("/Users/danny/src/dannyota/banhmi/deploy/eval/golden.json"))]
m.encode(qs[:3], prompt=spec.query_prompt)
lat = []
for q in qs:
    t = time.perf_counter(); m.encode([q], prompt=spec.query_prompt); lat.append((time.perf_counter() - t) * 1000)
params = sum(p.numel() for p in m.model.parameters())
print(json.dumps({"model": spec.key, "params_M": round(params / 1e6), "fp32_weights_GB": round(params * 4 / 1e9, 2),
                  "load_s": round(load, 1), "rss_GB": round(proc.memory_info().rss / 1e9, 2),
                  "p50_ms": round(float(np.percentile(lat, 50)), 1), "p95_ms": round(float(np.percentile(lat, 95)), 1)}))
