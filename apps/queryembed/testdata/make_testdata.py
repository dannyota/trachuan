"""Generate small native test fixtures without downloading model weights."""

from pathlib import Path

import numpy as np
import onnx
from onnx import TensorProto, helper, numpy_helper
from tokenizers import Tokenizer, models, pre_tokenizers, processors


def write_model(name: str, graph: onnx.GraphProto, *, external: bool = False) -> None:
    # These graphs need only IR 8; ONNX's newest default can exceed the runtime.
    model = helper.make_model(
        graph, ir_version=8, opset_imports=[helper.make_opsetid("", 18)]
    )
    onnx.checker.check_model(model)
    directory = Path(__file__).parent
    if external:
        (directory / "external.data").unlink(missing_ok=True)
        onnx.save_model(
            model,
            directory / name,
            save_as_external_data=True,
            all_tensors_to_one_file=True,
            location="external.data",
            size_threshold=64,
        )
    else:
        onnx.save(model, directory / name)


def main() -> None:
    directory = Path(__file__).parent
    vocabulary = {
        "<unk>": 0,
        "<s>": 1,
        "</s>": 2,
        "hello": 3,
        "world": 4,
        "law": 5,
        "vi": 6,
        "test": 7,
    }
    tokenizer = Tokenizer(models.WordLevel(vocabulary, unk_token="<unk>"))
    tokenizer.pre_tokenizer = pre_tokenizers.Whitespace()
    tokenizer.post_processor = processors.TemplateProcessing(
        single="<s> $A </s>", special_tokens=[("<s>", 1), ("</s>", 2)]
    )
    tokenizer.save(str(directory / "tokenizer.json"))

    inputs = [
        helper.make_tensor_value_info(n, TensorProto.INT64, ["batch", "sequence"])
        for n in ("input_ids", "attention_mask")
    ]
    output = helper.make_tensor_value_info(
        "embeddings", TensorProto.FLOAT, ["batch", "sequence", 8]
    )
    table = np.array(
        [
            [1] * 8,
            list(range(1, 9)),
            list(range(8, 0, -1)),
            [float("nan")] * 8,
            [float("inf")] * 8,
            [0] * 8,
            [np.finfo(np.float32).max] * 8,
            [2] * 8,
        ],
        dtype=np.float32,
    )
    initializers = [
        numpy_helper.from_array(table, "table"),
        numpy_helper.from_array(np.array([2], dtype=np.int64), "axis"),
    ]
    nodes = [
        helper.make_node("Gather", ["table", "input_ids"], ["lookup"]),
        helper.make_node("Cast", ["attention_mask"], ["mask"], to=TensorProto.FLOAT),
        helper.make_node("Unsqueeze", ["mask", "axis"], ["expanded_mask"]),
        helper.make_node("Mul", ["lookup", "expanded_mask"], ["embeddings"]),
    ]
    graph = helper.make_graph(nodes, "embedding_lookup", inputs, [output], initializers)
    write_model("tiny.onnx", graph)
    write_model("external.onnx", graph, external=True)
    wrong = onnx.GraphProto()
    wrong.CopyFrom(graph)
    wrong.input[0].name = "wrong_ids"
    wrong.node[0].input[1] = "wrong_ids"
    write_model("wrong_inputs.onnx", wrong)
    wrong.CopyFrom(graph)
    wrong.node.append(
        helper.make_node("Cast", ["embeddings"], ["integers"], to=TensorProto.INT64)
    )
    wrong.output[0].name = "integers"
    wrong.output[0].type.tensor_type.elem_type = TensorProto.INT64
    write_model("wrong_output.onnx", wrong)

    # A billion serial iterations take several seconds even on a fast CPU.
    body = helper.make_graph(
        [
            helper.make_node("Identity", ["condition"], ["continue"]),
            helper.make_node("Add", ["value", "one"], ["next"]),
        ],
        "loop_body",
        [
            helper.make_tensor_value_info("iteration", TensorProto.INT64, []),
            helper.make_tensor_value_info("condition", TensorProto.BOOL, []),
            helper.make_tensor_value_info("value", TensorProto.FLOAT, []),
        ],
        [
            helper.make_tensor_value_info("continue", TensorProto.BOOL, []),
            helper.make_tensor_value_info("next", TensorProto.FLOAT, []),
        ],
        [helper.make_tensor("one", TensorProto.FLOAT, [], [1])],
    )
    graph.node.extend(
        [
            helper.make_node("GatherND", ["input_ids", "first"], ["first_id"]),
            helper.make_node("Mul", ["first_id", "trips"], ["iterations"]),
            helper.make_node(
                "Loop", ["iterations", "condition", "start"], ["offset"], body=body
            ),
        ]
    )
    graph.node.append(helper.make_node("Add", ["embeddings", "offset"], ["result"]))
    graph.output[0].name = "result"
    graph.initializer.extend(
        [
            helper.make_tensor("trips", TensorProto.INT64, [], [1_000_000_000]),
            helper.make_tensor("first", TensorProto.INT64, [2], [0, 0]),
            helper.make_tensor("condition", TensorProto.BOOL, [], [True]),
            helper.make_tensor("start", TensorProto.FLOAT, [], [0]),
        ]
    )
    write_model("cancel.onnx", graph)


if __name__ == "__main__":
    main()
