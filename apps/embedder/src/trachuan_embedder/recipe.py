"""Stable embedding recipe reports."""

import hashlib
import json


def recipe_hash(recipe: dict) -> str:
    return hashlib.sha256(
        json.dumps(recipe, ensure_ascii=False, separators=(",", ":")).encode()
    ).hexdigest()


def build_recipe(manifest: dict, mode: str) -> dict:
    import importlib.metadata
    import platform

    files = manifest["files"]
    tokenizer = [
        item
        for item in files
        if "token" in item["path"] or item["path"].endswith("sentencepiece.bpe.model")
    ]
    weights = [
        item
        for item in files
        if (
            item["path"].startswith("onnx/")
            and item["path"].endswith((".onnx", ".onnx_data", ".data"))
            if mode == "query"
            else item["path"] in ("config.json", "model.safetensors")
        )
    ]
    if not tokenizer or not weights:
        raise ValueError("bundle lacks tokenizer or weights")

    def manifest_hash(items: list) -> str:
        return hashlib.sha256(
            json.dumps(items, ensure_ascii=False, separators=(",", ":"), sort_keys=True).encode()
        ).hexdigest()

    return {
        "model": manifest["model"],
        "revision": manifest["revision"],
        "resolved_revision": manifest["resolved_revision"],
        "tokenizer_revision": manifest["resolved_revision"],
        "tokenizer_sha256": manifest_hash(tokenizer),
        "weights_sha256": manifest_hash(weights),
        "dimensions": 1024,
        "pooling": "cls",
        "normalization": "l2-float32",
        "input": "raw",
        "runtime": mode,
        "backend": "onnxruntime-cpu" if mode == "query" else "torch-mps",
        "precision": "fp32",
        "token_cap": 256 if mode == "query" else 2048,
        "special_tokens": True,
        "truncation": False,
        "language": "python " + platform.python_version(),
        "tokenizer_library": "transformers " + importlib.metadata.version("transformers"),
        "runtime_library": (
            "onnxruntime " + importlib.metadata.version("onnxruntime")
            if mode == "query"
            else "torch " + importlib.metadata.version("torch")
        ),
    }
