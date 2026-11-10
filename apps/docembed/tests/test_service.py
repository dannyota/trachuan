import hashlib
import logging

import numpy as np
import pytest
from fastapi.testclient import TestClient

from trachuan_docembed.app import create_app
from trachuan_docembed.recipe import recipe_hash


class Tokenizer:
    def encode(self, texts):
        return [[1] * (len(text) + 2) for text in texts]


class Runtime:
    def __init__(self, values=None):
        self.values = values
        self.calls = 0

    def embed(self, tokens):
        self.calls += 1
        return np.asarray(self.values if self.values is not None else [[3, 4]] * len(tokens))


@pytest.fixture
def service():
    runtime = Runtime()
    app = create_app(Tokenizer(), runtime, {"dimensions": 2}, ready=True)
    return TestClient(app), runtime, app.state.recipe_hash


def test_recipe_hash_fixed_field_json():
    recipe = {"model": "a", "dimensions": 2}
    expected = hashlib.sha256(b'{"model":"a","dimensions":2}').hexdigest()
    assert recipe_hash(recipe) == expected


def test_tokenize_counts_without_truncation(service):
    client, _, recipe = service
    response = client.post("/v1/tokenize", json={"recipe_hash": recipe, "texts": ["a" * 2050]})
    assert response.json()["counts"] == [2052]


def test_embed_rejects_batch_when_one_input_over_cap(service):
    client, runtime, recipe = service
    response = client.post("/v1/embed", json={"recipe_hash": recipe, "texts": ["a", "b" * 2047]})
    assert response.status_code == 422
    assert response.json()["error"]["index"] == 1
    assert response.json()["error"]["token_count"] == 2049
    assert runtime.calls == 0


def test_embed_returns_float32_l2_normalized(service):
    client, _, recipe = service
    response = client.post("/v1/embed", json={"recipe_hash": recipe, "texts": ["a"]})
    vector = np.asarray(response.json()["data"][0]["embedding"], dtype=np.float32)
    assert np.allclose(vector, [0.6, 0.8])
    assert np.linalg.norm(vector) == pytest.approx(1)


@pytest.mark.parametrize("values", [[[0, 0]], [[float("nan"), 1]], [[float("inf"), 1]]])
def test_embed_rejects_non_finite_or_zero_norm(values):
    app = create_app(Tokenizer(), Runtime(values), {"dimensions": 2}, ready=True)
    client = TestClient(app)
    response = client.post("/v1/embed", json={"recipe_hash": app.state.recipe_hash, "texts": ["x"]})
    assert response.status_code == 500


def test_recipe_mismatch_409(service):
    client, _, _ = service
    assert client.post("/v1/embed", json={"recipe_hash": "bad", "texts": ["a"]}).status_code == 409


def test_unknown_field_and_duplicate_key_400(service):
    client, _, recipe = service
    assert (
        client.post(
            "/v1/embed", json={"recipe_hash": recipe, "texts": ["a"], "extra": 1}
        ).status_code
        == 400
    )
    assert (
        client.post(
            "/v1/embed",
            content='{"texts":[],"texts":[]}',
            headers={"content-type": "application/json"},
        ).status_code
        == 400
    )


def test_body_over_cap_413_before_parse(service):
    client, _, _ = service
    assert (
        client.post(
            "/v1/embed",
            content=b"x" * (1024 * 1024 + 1),
            headers={"content-type": "application/json"},
        ).status_code
        == 413
    )


def test_wrong_media_type_415(service):
    client, _, _ = service
    assert (
        client.post("/v1/embed", content="{}", headers={"content-type": "text/plain"}).status_code
        == 415
    )


def test_readyz_503_until_bundle_verified():
    app = create_app(Tokenizer(), Runtime(), {"dimensions": 2}, ready=False)
    assert TestClient(app).get("/readyz").status_code == 503


def test_no_text_in_logs(service, caplog):
    client, _, recipe = service
    marker = "sensitive-unique-marker-48293"
    with caplog.at_level(logging.DEBUG):
        client.post("/v1/embed", json={"recipe_hash": recipe, "texts": [marker]})
    assert marker not in caplog.text
