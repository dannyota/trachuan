import sys

import numpy as np
import pytest

from trachuan_docembed import __main__ as entry
from trachuan_docembed import torch_runtime


def test_document_service_starts_on_loopback_without_runtime_flag(monkeypatch, tmp_path):
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

    listener = []
    monkeypatch.setattr(sys, "argv", ["embedder", "--model-dir", str(tmp_path)])
    monkeypatch.setattr(
        entry,
        "verify_bundle",
        lambda path: {"model": "AITeamVN/Vietnamese_Embedding_v2", "revision": "18b44161e041"},
    )
    monkeypatch.setattr(entry, "Tokenizer", Tokenizer)
    monkeypatch.setattr(torch_runtime, "TorchRuntime", Runtime)
    monkeypatch.setattr(entry, "build_recipe", lambda *args: {"dimensions": 2})
    monkeypatch.setattr(entry, "create_app", lambda *args, **kwargs: None)
    monkeypatch.setattr(entry.uvicorn, "run", lambda app, **kwargs: listener.append(kwargs))
    entry.main()
    assert listener[0]["host"] == "127.0.0.1"
    assert listener[0]["port"] == 8092


def test_query_runtime_is_refused(monkeypatch, tmp_path):
    monkeypatch.setattr(
        sys, "argv", ["embedder", "--runtime", "query", "--model-dir", str(tmp_path)]
    )
    with pytest.raises(SystemExit) as exc:
        entry.main()
    assert exc.value.code == 2


def test_non_loopback_host_is_refused(monkeypatch, tmp_path):
    monkeypatch.setattr(
        sys, "argv", ["embedder", "--host", "0.0.0.0", "--model-dir", str(tmp_path)]
    )
    with pytest.raises(SystemExit) as exc:
        entry.main()
    assert exc.value.code == 2
