"""Shared vector validation and normalization."""

import numpy as np


def normalize(values: object) -> np.ndarray:
    vectors = np.asarray(values, dtype=np.float32)
    if vectors.ndim == 3:
        vectors = vectors[:, 0, :]
    if vectors.ndim != 2 or not np.isfinite(vectors).all():
        raise ValueError("invalid vectors")
    norms = np.linalg.norm(vectors, axis=1, keepdims=True)
    if not np.isfinite(norms).all() or (norms == 0).any():
        raise ValueError("invalid vector norms")
    return vectors / norms
