import numpy as np
import onnx
import pytest
from onnx import TensorProto, helper

from trachuan_embedder.bundle import create_bundle, verify_bundle
from trachuan_embedder.onnx_runtime import OnnxRuntime
from trachuan_embedder.torch_runtime import TorchRuntime


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


def test_onnx_cls_selection_and_normalization(tmp_path):
    source = helper.make_tensor_value_info("input_ids", TensorProto.FLOAT, [1, 2, 2])
    result = helper.make_tensor_value_info("hidden", TensorProto.FLOAT, [1, 2, 2])
    graph = helper.make_graph(
        [helper.make_node("Identity", ["input_ids"], ["hidden"])], "tiny", [source], [result]
    )
    model = helper.make_model(graph, opset_imports=[helper.make_opsetid("", 13)], ir_version=10)
    path = tmp_path / "model.onnx"
    onnx.save(model, path)
    runtime = OnnxRuntime(path)
    vector = runtime.embed({"input_ids": np.array([[[3, 4], [9, 9]]], dtype=np.float32)})
    assert np.allclose(vector, [[0.6, 0.8]])


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
