import importlib.metadata
import platform

import pytest

from trachuan_embedder.bundle import create_bundle
from trachuan_embedder.recipe import build_recipe, recipe_hash


@pytest.fixture
def bundle(tmp_path):
    for name in (
        "config.json",
        "model.safetensors",
        "tokenizer.json",
        "onnx/model.onnx",
        "onnx/model.onnx_data",
        "onnx/extra.data",
    ):
        path = tmp_path / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(name)
    return tmp_path


@pytest.fixture(autouse=True)
def torch_version(monkeypatch):
    # The default environment has no PyTorch; the document recipe only reads
    # its version, so a fixed one stands in when the package is missing.
    real = importlib.metadata.version

    def version(name):
        try:
            return real(name)
        except importlib.metadata.PackageNotFoundError:
            if name == "torch":
                return "0.0.0"
            raise

    monkeypatch.setattr(importlib.metadata, "version", version)


def manifest(path):
    return create_bundle(path, "model", "abc", "abc" + "0" * 37)


@pytest.mark.parametrize("mode,library", [("document", "torch"), ("query", "onnxruntime")])
def test_recipe_reports_loaded_versions_and_precision(bundle, mode, library):
    recipe = build_recipe(manifest(bundle), mode)
    assert recipe["precision"] == ("fp16" if mode == "document" else "fp32")
    assert list(recipe)[-3:] == ["language", "tokenizer_library", "runtime_library"]
    assert recipe["language"] == "python " + platform.python_version()
    assert recipe["tokenizer_library"] == "transformers " + importlib.metadata.version(
        "transformers"
    )
    assert recipe["runtime_library"] == library + " " + importlib.metadata.version(library)


@pytest.mark.parametrize(
    "path,changed",
    [
        ("onnx/model.onnx", {"query"}),
        ("onnx/model.onnx_data", {"query"}),
        ("onnx/extra.data", set()),
        ("model.safetensors", {"document"}),
        ("config.json", {"document"}),
        ("tokenizer.json", {"query", "document"}),
    ],
)
def test_recipe_hash_tracks_only_runtime_files(bundle, path, changed):
    before = manifest(bundle)
    (bundle / path).write_text("changed")
    after = manifest(bundle)
    for mode in ("query", "document"):
        assert (
            recipe_hash(build_recipe(before, mode)) != recipe_hash(build_recipe(after, mode))
        ) == (mode in changed)


def test_query_recipe_ignores_int8_model_and_data(bundle):
    before = build_recipe(manifest(bundle), "query")
    for name in ("model_int8.onnx", "model_int8.onnx_data"):
        (bundle / "onnx" / name).write_text(name)
    assert build_recipe(manifest(bundle), "query") == before


def test_query_recipe_tracks_dot_data(bundle):
    path = bundle / "onnx/model.onnx.data"
    path.write_text("first")
    before = build_recipe(manifest(bundle), "query")
    path.write_text("second")
    assert build_recipe(manifest(bundle), "query")["weights_sha256"] != before["weights_sha256"]
