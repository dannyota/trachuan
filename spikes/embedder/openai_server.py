"""Throwaway spike: OpenAI-compatible /embeddings server for Vietnamese_Embedding_v2.

banhmi's evaluator sends queries with the Qwen3 instruction prefix; this server
strips it and embeds the raw question with the ADR 0009 query recipe (ONNX FP32,
CLS pooling, L2, at most 256 tokens), so banhmi's whole retrieval pipeline can
be scored with vi_v2 vectors and nothing else changed.
"""

import sys

import uvicorn
from fastapi import FastAPI
from pydantic import BaseModel

from v2_onnx import DIR, Onnx

PREFIX_END = "\nQuery:"

app = FastAPI()
model = Onnx(str(DIR / "model.onnx"))


class Request(BaseModel):
    model: str = ""
    input: list[str]


def strip(text: str) -> str:
    if text.startswith("Instruct:") and PREFIX_END in text:
        return text.split(PREFIX_END, 1)[1].strip()
    return text


@app.post("/embeddings")
def embeddings(req: Request):
    vectors = model.encode([strip(t) for t in req.input], 256)
    return {"model": req.model, "data": [{"embedding": v.tolist(), "index": i} for i, v in enumerate(vectors)]}


if __name__ == "__main__":
    uvicorn.run(app, host="127.0.0.1", port=int(sys.argv[1]) if len(sys.argv) > 1 else 18080, log_level="warning")
