import hashlib
import runpy
import sys
from pathlib import Path

import onnx
import pytest
from onnx import TensorProto, helper

from trachuan_docembed.bundle import verify_bundle

SCRIPT = Path(__file__).resolve().parents[1] / "scripts/make_bundle.py"
REVISION = "a" * 40


def run_make_bundle(monkeypatch, model_dir):
    monkeypatch.setattr(
        sys, "argv", [str(SCRIPT), str(model_dir), "--model", "model", "--revision", REVISION]
    )
    runpy.run_path(str(SCRIPT), run_name="__main__")


def test_make_bundle_quantizes_before_manifest(monkeypatch, tmp_path):
    model_dir = tmp_path / "onnx"
    model_dir.mkdir()
    graph = helper.make_graph(
        [helper.make_node("MatMul", ["input", "weight"], ["output"])],
        "tiny",
        [helper.make_tensor_value_info("input", TensorProto.FLOAT, [1, 2])],
        [helper.make_tensor_value_info("output", TensorProto.FLOAT, [1, 2])],
        [helper.make_tensor("weight", TensorProto.FLOAT, [2, 2], [1.0, 2.0, 3.0, 4.0])],
    )
    onnx.save(
        helper.make_model(graph, opset_imports=[helper.make_opsetid("", 13)]),
        model_dir / "model.onnx",
    )
    run_make_bundle(monkeypatch, tmp_path)
    output = model_dir / "model_int8.onnx"
    manifest = verify_bundle(tmp_path)
    entry = next(item for item in manifest["files"] if item["path"] == "onnx/model_int8.onnx")
    assert entry["size"] == output.stat().st_size
    assert entry["sha256"] == hashlib.sha256(output.read_bytes()).hexdigest()
    model = onnx.load(output)
    assert any(node.op_type == "MatMulInteger" for node in model.graph.node)
    initializers = {item.name: item for item in model.graph.initializer}
    assert initializers["weight_quantized"].data_type == TensorProto.INT8
    assert list(initializers["weight_scale"].dims) == [2]
    assert sorted(path.name for path in model_dir.iterdir()) == ["model.onnx", "model_int8.onnx"]


def test_make_bundle_failed_rerun_keeps_previous_model(monkeypatch, tmp_path):
    test_make_bundle_quantizes_before_manifest(monkeypatch, tmp_path)
    before = {
        path.relative_to(tmp_path): path.read_bytes()
        for path in tmp_path.rglob("*")
        if path.is_file()
    }

    def interrupted(model_input, model_output, **_):
        Path(model_output).write_bytes(b"partial")
        raise RuntimeError("interrupted")

    import onnxruntime.quantization

    monkeypatch.setattr(onnxruntime.quantization, "quantize_dynamic", interrupted)
    with pytest.raises(RuntimeError):
        run_make_bundle(monkeypatch, tmp_path)
    after = {
        path.relative_to(tmp_path): path.read_bytes()
        for path in tmp_path.rglob("*")
        if path.is_file()
    }
    assert after == before
    verify_bundle(tmp_path)
    assert not [path for path in tmp_path.parent.iterdir() if path.name.startswith("tmp")]


def test_make_bundle_refuses_missing_model(monkeypatch, tmp_path):
    with pytest.raises(FileNotFoundError):
        run_make_bundle(monkeypatch, tmp_path)
    assert not (tmp_path / "bundle.json").exists()
