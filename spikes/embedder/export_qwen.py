"""Throwaway spike: export Qwen3-Embedding-0.6B to ONNX FP32 and dynamic INT8."""

import subprocess
import sys
from pathlib import Path

import onnx
from onnxruntime.quantization import QuantType, quantize_dynamic
from onnxruntime.quantization.matmul_nbits_quantizer import MatMulNBitsQuantizer

MODEL_ID = "Qwen/Qwen3-Embedding-0.6B"


def export(out_dir: Path) -> None:
    """Write model.onnx (FP32) and model_int8.onnx (dynamic INT8 weights) to out_dir."""
    out_dir.mkdir(parents=True, exist_ok=True)
    fp32 = out_dir / "model.onnx"
    if not fp32.exists():
        subprocess.run(
            ["optimum-cli", "export", "onnx", "-m", MODEL_ID, "--task", "feature-extraction",
             "--opset", "17", str(out_dir)],
            check=True,
        )
    int8 = out_dir / "model_int8.onnx"
    quantize_dynamic(
        model_input=str(fp32),
        model_output=str(int8),
        weight_type=QuantType.QInt8,
        per_channel=True,
        use_external_data_format=True,
    )


def weight_only(out_dir: Path, bits: int, accuracy_level: int | None = None) -> Path:
    """Write model_q{bits}w[_a{level}].onnx: weight-only block quantization, activations kept in float."""
    name = f"model_q{bits}w" + (f"_a{accuracy_level}" if accuracy_level else "") + ".onnx"
    model = onnx.load(str(out_dir / "model.onnx"), load_external_data=True)
    quant = MatMulNBitsQuantizer(model, bits=bits, block_size=128, is_symmetric=True, accuracy_level=accuracy_level)
    quant.process()
    out = out_dir / name
    onnx.save_model(quant.model.model, str(out), save_as_external_data=True, location=name + ".data")
    return out


if __name__ == "__main__":
    target = Path(sys.argv[1] if len(sys.argv) > 1 else "../../work/models/qwen3-0.6b")
    if len(sys.argv) > 2:
        level = int(sys.argv[3]) if len(sys.argv) > 3 else None
        print(weight_only(target, int(sys.argv[2]), level))
    else:
        export(target)
