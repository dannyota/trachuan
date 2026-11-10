# Python

Read for `apps/embedder`. Python is used only for document embedding on the MacBook (ADR 0011); the query embedder is Go; spikes under `spikes/` are throwaway and follow nothing here.

- uv manages the project: `.python-version` pins Python 3.14, `uv.lock` is committed, commands run as `uv run --frozen ...`. Add a dependency with `uv add`, never by editing the lock. The `document` dependency group holds PyTorch; the `dev` group holds pytest and ruff.
- Style: `ruff format` and `ruff check` (rules E, F, I, B, UP, line length 100). Type hints on every public function. One concern per module, at most 400 lines, named after the concern; `__main__.py` only wires startup.
- Tests: pytest under `tests/`, files `test_<subject>.py`, functions `test_<behavior>`. Inject tokenizer, runtime and clock fakes; tests never load the production model, download files or use the network, and never wait on real time beyond 100 ms. A real-model smoke is an opt-in make target the primary runs.
- Never log request bodies, input text or vectors; log code, runtime, counts and latency only. No `trust_remote_code`, no downloads at startup, no environment values in responses.
- Checks: `make embedder-test` (pytest) and `make pre-push` (ruff when uv is installed); CI runs both when `apps/embedder` changes.
