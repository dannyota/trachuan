"""Wire the verified local bundle to the HTTP server."""

import argparse
from pathlib import Path

import uvicorn

from .app import create_app
from .bundle import verify_bundle
from .recipe import build_recipe
from .tokenizer import Tokenizer


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--model-dir", type=Path, required=True)
    parser.add_argument("--host")
    parser.add_argument("--port", type=int)
    args = parser.parse_args()
    if args.host not in (None, "127.0.0.1"):
        parser.error("document runtime requires 127.0.0.1")
    manifest = verify_bundle(args.model_dir)
    if (
        manifest["model"] != "AITeamVN/Vietnamese_Embedding_v2"
        or manifest["revision"] != "18b44161e041"
    ):
        raise ValueError("bundle does not match the dense recipe")
    tokenizer = Tokenizer(args.model_dir)
    from .torch_runtime import TorchRuntime

    runtime = TorchRuntime(args.model_dir)
    recipe = build_recipe(manifest)
    tokens = tokenizer.encode(["warmup"])
    vectors = runtime.embed(tokenizer.pad(tokens))
    if vectors.shape != (1, recipe["dimensions"]):
        raise ValueError("warmup failed")
    app = create_app(tokenizer, runtime, recipe, ready=True)
    uvicorn.run(
        app,
        host=args.host or "127.0.0.1",
        port=args.port or 8092,
        access_log=False,
    )


if __name__ == "__main__":
    main()
