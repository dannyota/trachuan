import importlib.metadata
import platform

import pytest

from trachuan_docembed.bundle import create_bundle
from trachuan_docembed.recipe import build_recipe, recipe_hash


@pytest.fixture
def bundle(tmp_path):
    for name in (
        "config.json",
        "model.safetensors",
        "tokenizer.json",
        "onnx/model.onnx",
        "onnx/model_int8.onnx",
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


def test_recipe_reports_loaded_versions_and_precision(bundle):
    recipe = build_recipe(manifest(bundle))
    assert recipe["precision"] == "fp16"
    assert recipe["runtime"] == "document"
    assert recipe["backend"] == "torch-mps"
    assert recipe["token_cap"] == 2048
    assert list(recipe)[-3:] == ["language", "tokenizer_library", "runtime_library"]
    assert recipe["language"] == "python " + platform.python_version()
    assert recipe["tokenizer_library"] == "transformers " + importlib.metadata.version(
        "transformers"
    )
    assert recipe["runtime_library"] == "torch " + importlib.metadata.version("torch")


@pytest.mark.parametrize(
    "path,changed",
    [
        ("onnx/model.onnx", False),
        ("onnx/model.onnx_data", False),
        ("onnx/extra.data", False),
        ("onnx/model_int8.onnx", False),
        ("model.safetensors", True),
        ("config.json", True),
        ("tokenizer.json", True),
    ],
)
def test_recipe_hash_tracks_only_runtime_files(bundle, path, changed):
    before = manifest(bundle)
    (bundle / path).write_text("changed")
    after = manifest(bundle)
    assert (recipe_hash(build_recipe(before)) != recipe_hash(build_recipe(after))) == changed
