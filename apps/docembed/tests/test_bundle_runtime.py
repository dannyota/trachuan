import pytest

from trachuan_docembed.bundle import create_bundle, verify_bundle
from trachuan_docembed.torch_runtime import TorchRuntime


def test_bundle_hash_mismatch_refuses_ready(tmp_path):
    (tmp_path / "config.json").write_text("first")
    create_bundle(tmp_path, "model", "abc", "abc" + "0" * 37)
    (tmp_path / "config.json").write_text("other")
    with pytest.raises(ValueError):
        verify_bundle(tmp_path)


def test_document_mode_refuses_without_mps(monkeypatch, tmp_path):
    torch = pytest.importorskip("torch")
    monkeypatch.setattr(torch.backends.mps, "is_available", lambda: False)
    with pytest.raises(RuntimeError):
        TorchRuntime(tmp_path)


def test_document_runtime_loads_float16_on_mps(monkeypatch, tmp_path):
    torch = pytest.importorskip("torch")
    from transformers import AutoModel

    class FakeModel:
        def to(self, device):
            assert device == "mps"
            return self

        def eval(self):
            return self

    def load(path, *, local_files_only, trust_remote_code, dtype):
        assert path == str(tmp_path)
        assert local_files_only is True
        assert trust_remote_code is False
        assert dtype == torch.float16
        return FakeModel()

    monkeypatch.setattr(torch.backends.mps, "is_available", lambda: True)
    monkeypatch.setattr(AutoModel, "from_pretrained", load)
    TorchRuntime(tmp_path)
