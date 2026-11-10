"""Write a model bundle manifest without downloading files."""

import argparse
import os
import tempfile
from pathlib import Path

from onnxruntime.quantization import QuantType, quantize_dynamic

from trachuan_docembed.bundle import create_bundle


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("model_dir", type=Path)
    parser.add_argument("--model", required=True)
    parser.add_argument("--revision", required=True, help="Full resolved model commit")
    args = parser.parse_args()
    model_input = args.model_dir / "onnx/model.onnx"
    if not model_input.is_file():
        raise FileNotFoundError(model_input)
    # Quantize outside the bundle and replace the model only after success, so
    # an interrupted run leaves the previous model and manifest intact.
    with tempfile.TemporaryDirectory(dir=args.model_dir.resolve().parent) as work:
        partial = Path(work) / "model_int8.onnx"
        quantize_dynamic(
            model_input=model_input,
            model_output=partial,
            weight_type=QuantType.QInt8,
            per_channel=True,
        )
        os.replace(partial, args.model_dir / "onnx/model_int8.onnx")
    create_bundle(args.model_dir, args.model, args.revision[:12], args.revision)


if __name__ == "__main__":
    main()
