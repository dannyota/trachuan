"""CPU ONNX inference."""

from pathlib import Path

import numpy as np
import onnxruntime as ort

from .runtime import normalize


class OnnxRuntime:
    def __init__(self, path: Path):
        options = ort.SessionOptions()
        options.intra_op_num_threads = 2
        options.inter_op_num_threads = 1
        options.execution_mode = ort.ExecutionMode.ORT_SEQUENTIAL
        self.session = ort.InferenceSession(str(path), options, providers=["CPUExecutionProvider"])

    def embed(self, inputs: dict) -> np.ndarray:
        feeds = {item.name: inputs[item.name] for item in self.session.get_inputs()}
        return normalize(self.session.run(None, feeds)[0])
