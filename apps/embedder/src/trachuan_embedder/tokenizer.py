"""Offline tokenizer adapter."""

from pathlib import Path

from transformers import AutoTokenizer


class Tokenizer:
    def __init__(self, model_dir: Path):
        self.tokenizer = AutoTokenizer.from_pretrained(
            str(model_dir), local_files_only=True, trust_remote_code=False
        )

    def encode(self, texts: list[str]) -> list[list[int]]:
        return self.tokenizer(texts, add_special_tokens=True, truncation=False, padding=False)[
            "input_ids"
        ]

    def pad(self, tokens: list[list[int]]) -> dict:
        return dict(self.tokenizer.pad({"input_ids": tokens}, padding=True, return_tensors="np"))
