"""HTTP contract for the local embedding service."""

import asyncio
import json

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse, Response

from .admission import Admission, Busy, Deadline
from .recipe import recipe_hash
from .runtime import normalize


class Failure(Exception):
    def __init__(self, status: int, code: str, index: int | None = None, count: int | None = None):
        self.status, self.code, self.index, self.count = status, code, index, count


def pairs(items: list) -> dict:
    result = {}
    for key, value in items:
        if key in result:
            raise ValueError("duplicate key")
        result[key] = value
    return result


def create_app(
    mode: str, tokenizer: object, runtime: object, recipe: dict, *, ready: bool = False
) -> FastAPI:
    app = FastAPI(docs_url=None, redoc_url=None, openapi_url=None)
    app.state.ready = ready
    app.state.recipe_hash = recipe_hash(recipe)
    cap, body_cap, batch, active, queued, wait, deadline = (
        (256, 64000, 1, 2, 8, 0.25, 2.0)
        if mode == "query"
        else (2048, 1048576, 16, 1, 1, 30.0, 120.0)
    )
    admission = Admission(active, queued, wait, deadline)

    @app.get("/healthz")
    async def health() -> dict:
        return {"status": "alive"}

    @app.get("/readyz")
    async def readiness() -> object:
        if not app.state.ready:
            return error(Failure(503, "not_ready"))
        return {"status": "ready", "recipe_hash": app.state.recipe_hash}

    @app.get("/v1/recipe")
    async def report() -> object:
        if not app.state.ready:
            return error(Failure(503, "not_ready"))
        return {"protocol_version": 1, "recipe_hash": app.state.recipe_hash, "recipe": recipe}

    def work(texts: list[str], embedding: bool) -> bytes:
        tokens = tokenizer.encode(texts)
        counts = [len(item) for item in tokens]
        if not embedding:
            return json.dumps({"recipe_hash": app.state.recipe_hash, "counts": counts}).encode()
        for index, count in enumerate(counts):
            if count > cap:
                raise Failure(422, "input_too_long", index, count)
        inputs = tokenizer.pad(tokens) if hasattr(tokenizer, "pad") else tokens
        vectors = normalize(runtime.embed(inputs))
        if vectors.shape != (len(texts), recipe["dimensions"]):
            raise ValueError("invalid dimensions")
        result = {
            "recipe_hash": app.state.recipe_hash,
            "dimensions": recipe["dimensions"],
            "data": [
                {"index": index, "token_count": counts[index], "embedding": vector.tolist()}
                for index, vector in enumerate(vectors)
            ],
        }
        return json.dumps(result, allow_nan=False, separators=(",", ":")).encode()

    async def process(request: Request, embedding: bool) -> Response:
        if not app.state.ready:
            return error(Failure(503, "not_ready"))
        if (
            request.headers.get("content-type", "").split(";", 1)[0].strip().lower()
            != "application/json"
        ):
            return error(Failure(415, "unsupported_media_type"))
        try:
            async with asyncio.timeout(deadline):
                body = bytearray()
                async for chunk in request.stream():
                    body.extend(chunk)
                    if len(body) > body_cap:
                        raise Failure(413, "body_too_large")
                try:
                    data = json.loads(
                        body.decode("utf-8"),
                        object_pairs_hook=pairs,
                        parse_constant=lambda _: (_ for _ in ()).throw(ValueError()),
                    )
                except (ValueError, UnicodeDecodeError):
                    raise Failure(400, "invalid_request") from None
                if (
                    not isinstance(data, dict)
                    or set(data) != {"recipe_hash", "texts"}
                    or not isinstance(data["recipe_hash"], str)
                ):
                    raise Failure(400, "invalid_request")
                texts = data["texts"]
                if (
                    not isinstance(texts, list)
                    or not 1 <= len(texts) <= batch
                    or any(not isinstance(text, str) or not text.strip() for text in texts)
                ):
                    raise Failure(400, "invalid_request")
                if data["recipe_hash"] != app.state.recipe_hash:
                    raise Failure(409, "recipe_mismatch")
                result = await admission.run(lambda: work(texts, embedding))
                return Response(result, media_type="application/json")
        except Failure as exc:
            return error(exc)
        except Busy:
            return error(Failure(429, "busy"))
        except (Deadline, TimeoutError):
            return error(Failure(504, "deadline_exceeded"))
        except Exception:
            return error(Failure(500, "inference_failed"))

    @app.post("/v1/tokenize")
    async def tokenize(request: Request) -> Response:
        return await process(request, False)

    @app.post("/v1/embed")
    async def embed(request: Request) -> Response:
        return await process(request, True)

    return app


def error(failure: Failure) -> JSONResponse:
    return JSONResponse(
        status_code=failure.status,
        content={
            "error": {
                "code": failure.code,
                "message": failure.code.replace("_", " "),
                "index": failure.index,
                "token_count": failure.count,
            }
        },
    )
