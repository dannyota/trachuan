"""Throwaway spike: embedding backends sharing one recipe.

Qwen recipe (matches banhmi's stored vectors): documents are
strip(strip(prefix) + "\\n" + strip(content)) with no instruction; queries are
QWEN_QUERY_PREFIX + query; tokenizer.json with its post-processor; last
non-padding token pooled; L2 normalized; 1,024 dimensions.
"""

from pathlib import Path

import numpy as np

QWEN_QUERY_PREFIX = (
    "Instruct: Given a legal or regulatory query, retrieve relevant provisions and "
    "regulatory text that address the query\nQuery:"
)


def doc_text(prefix: str | None, content: str) -> str:
    return "\n".join([(prefix or "").strip(), content.strip()]).strip()


def _normalize(x: np.ndarray) -> np.ndarray:
    return x / np.linalg.norm(x, axis=1, keepdims=True)


class OnnxQwen:
    """Qwen3-Embedding through ONNX Runtime (FP32 or INT8 graph)."""

    def __init__(self, model_path: Path, threads: int = 0):
        import onnxruntime as ort
        from tokenizers import Tokenizer

        opts = ort.SessionOptions()
        if threads:
            opts.intra_op_num_threads = threads
        self.session = ort.InferenceSession(str(model_path), opts, providers=["CPUExecutionProvider"])
        self.tokenizer = Tokenizer.from_file(str(Path(model_path).parent / "tokenizer.json"))
        self.pad_id = self.tokenizer.token_to_id("<|endoftext|>")

    def encode(self, texts: list[str], max_len: int = 8192) -> np.ndarray:
        self.tokenizer.enable_truncation(max_length=max_len)
        encs = self.tokenizer.encode_batch(texts)
        width = max(len(e.ids) for e in encs)
        ids = np.full((len(encs), width), self.pad_id, dtype=np.int64)
        mask = np.zeros((len(encs), width), dtype=np.int64)
        for i, e in enumerate(encs):
            ids[i, : len(e.ids)] = e.ids
            mask[i, : len(e.ids)] = 1
        tokens = self.session.run(["token_embeddings"], {"input_ids": ids, "attention_mask": mask})[0]
        last = mask.sum(axis=1) - 1
        return _normalize(tokens[np.arange(len(encs)), last].astype(np.float32))


class TorchModel:
    """A sentence-transformers model on MPS or CPU; prompts are passed explicitly."""

    def __init__(self, model_id: str, device: str = "mps", fp16: bool = True, max_len: int = 8192,
                 revision: str | None = None, remote_code: bool = False):
        import torch
        from sentence_transformers import SentenceTransformer

        kwargs = {"dtype": torch.float16 if fp16 and device != "cpu" else torch.float32}
        self.model = SentenceTransformer(model_id, device=device, model_kwargs=kwargs, revision=revision,
                                         trust_remote_code=remote_code)
        self.model.max_seq_length = max_len
        self.tokenizer = self.model.tokenizer

    def encode(self, texts: list[str], max_len: int | None = None, batch_size: int = 16, prompt: str = "") -> np.ndarray:
        if max_len:
            self.model.max_seq_length = max_len
        vecs = self.model.encode(texts, batch_size=batch_size, prompt=prompt or None, convert_to_numpy=True)
        return _normalize(np.asarray(vecs, dtype=np.float32))

    def token_lengths(self, texts: list[str], prompt: str = "") -> list[int]:
        return [len(x) for x in self.tokenizer([prompt + t for t in texts], add_special_tokens=True)["input_ids"]]
