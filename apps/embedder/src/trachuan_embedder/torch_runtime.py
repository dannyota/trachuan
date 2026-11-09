"""MPS document inference."""

from pathlib import Path

import numpy as np

from .runtime import normalize


class TorchRuntime:
    def __init__(self, model_dir: Path):
        import torch
        from transformers import AutoModel

        if not torch.backends.mps.is_available():
            raise RuntimeError("MPS is unavailable")
        self.torch = torch
        self.model = (
            AutoModel.from_pretrained(
                str(model_dir), local_files_only=True, trust_remote_code=False, dtype=torch.float16
            )
            .to("mps")
            .eval()
        )

    def embed(self, inputs: dict) -> np.ndarray:
        tensors = {key: self.torch.as_tensor(value, device="mps") for key, value in inputs.items()}
        with self.torch.inference_mode():
            hidden = self.model(**tensors).last_hidden_state
        return normalize(hidden.float().cpu().numpy())
