import sys

import numpy as np
import pytest

from trachuan_embedder import __main__ as entry
from trachuan_embedder import torch_runtime


@pytest.mark.parametrize("mode", ["query", "document"])
def test_default_host_is_loopback(monkeypatch, tmp_path, mode):
    class Tokenizer:
        def __init__(self, path):
            pass

        def encode(self, texts):
            return texts

        def pad(self, tokens):
            return tokens

    class Runtime:
        def __init__(self, path):
            pass

        def embed(self, inputs):
            return np.zeros((1, 2))

    host = []
    monkeypatch.setattr(sys, "argv", ["embedder", "--runtime", mode, "--model-dir", str(tmp_path)])
    monkeypatch.setattr(
        entry,
        "verify_bundle",
        lambda path: {"model": "AITeamVN/Vietnamese_Embedding_v2", "revision": "18b44161e041"},
    )
    monkeypatch.setattr(entry, "Tokenizer", Tokenizer)
    monkeypatch.setattr(entry, "OnnxRuntime", Runtime)
    monkeypatch.setattr(torch_runtime, "TorchRuntime", Runtime)
    monkeypatch.setattr(entry, "build_recipe", lambda manifest, mode: {"dimensions": 2})
    monkeypatch.setattr(entry, "create_app", lambda *args, **kwargs: None)
    monkeypatch.setattr(entry.uvicorn, "run", lambda app, **kwargs: host.append(kwargs["host"]))
    entry.main()
    assert host == ["127.0.0.1"]
