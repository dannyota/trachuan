"""Write a model bundle manifest without downloading files."""

import argparse
from pathlib import Path

from trachuan_embedder.bundle import create_bundle


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("model_dir", type=Path)
    parser.add_argument("--model", required=True)
    parser.add_argument("--revision", required=True, help="Full resolved model commit")
    args = parser.parse_args()
    create_bundle(args.model_dir, args.model, args.revision[:12], args.revision)


if __name__ == "__main__":
    main()
