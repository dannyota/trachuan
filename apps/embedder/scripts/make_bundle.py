"""Write a model bundle manifest without downloading files."""

import argparse
from pathlib import Path

from onnxruntime.quantization import QuantType, quantize_dynamic

from trachuan_embedder.bundle import create_bundle


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("model_dir", type=Path)
    parser.add_argument("--model", required=True)
    parser.add_argument("--revision", required=True, help="Full resolved model commit")
    args = parser.parse_args()
    model_input = args.model_dir / "onnx/model.onnx"
    if not model_input.is_file():
        raise FileNotFoundError(model_input)
    quantize_dynamic(
        model_input=model_input,
        model_output=args.model_dir / "onnx/model_int8.onnx",
        weight_type=QuantType.QInt8,
        per_channel=True,
    )
    create_bundle(args.model_dir, args.model, args.revision[:12], args.revision)


if __name__ == "__main__":
    main()
