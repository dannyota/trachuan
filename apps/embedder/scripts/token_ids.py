"""Write tokenizer IDs for parity inputs without loading the embedding model."""

import argparse
import json
from pathlib import Path

from trachuan_embedder.tokenizer import Tokenizer


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("legacy", type=Path)
    parser.add_argument("edge", type=Path)
    parser.add_argument("--model-dir", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    try:
        legacy = json.loads(args.legacy.read_text(encoding="utf-8"))
        edges = json.loads(args.edge.read_text(encoding="utf-8"))
        texts = [case["question"] for case in legacy] + edges
        ids = Tokenizer(args.model_dir).encode(texts)
        args.out.write_text(json.dumps({"ids": ids}), encoding="utf-8")
    except Exception:
        raise SystemExit("Unable to write tokenizer IDs") from None


if __name__ == "__main__":
    main()
