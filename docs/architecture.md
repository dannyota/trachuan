# Architecture

What exists in the repository today. The intended shape of the whole system is
in the [system design](design/system.md); contracts live in the
[design notes](design/README.md) and [ADRs](adr/README.md).

## Corpus pipeline (`apps/corpus`)

One Go module, `danny.vn/trachuan`, at the repository root. The pipeline is one
binary, `corpus`, built from `apps/corpus/cmd/corpus`. It runs on the owner's
MacBook against native local services (`deploy/local`)
([runbook](runbooks/corpus-local.md)).

| Package            | Role                                                                                                                                |
| ------------------ | ----------------------------------------------------------------------------------------------------------------------------------- |
| `cli`              | Commands: `inventory`, `eval`, `migrate`, `worker`, `start`, `status`, `cancel`, `recover`, `gaps`, `accept-gaps`, `export-history` |
| `config`           | Environment settings (`CORPUS_*`), defaults from [budgets](design/budgets.md)                                                       |
| `run`              | Run lifecycle: start protocol, snapshot, cancel, recover, gap acceptance, publication check                                         |
| `workflow`         | `BuildRelease` workflow, stage registry, Temporal activity adapters                                                                 |
| `worker`           | Temporal worker wiring                                                                                                              |
| `stage`            | Stage contract, batch processing, fenced item commit, stage planning and closing                                                    |
| `stage/discover`   | VBPL listings and the expected document set                                                                                         |
| `stage/fetch`      | Detail, tree, diagram and attachments into a content-addressed bundle                                                               |
| `stage/extract`    | Provision tree into provisions with verbatim text                                                                                   |
| `stage/relate`     | Source relations and run-time target resolution                                                                                     |
| `stage/chunk`      | Provision-owned chunks and source spans                                                                                             |
| `stage/embed`      | Document embeddings                                                                                                                 |
| `stage/build`      | Corpus database, calibration and dump                                                                                               |
| `stage/eval`       | Evaluation artifacts and quality gaps                                                                                               |
| `evaluation`       | Fixtures, calibration, scoring and report caches                                                                                    |
| `artifact`         | Content-addressed artifact store with manifests and lock files                                                                      |
| `checkpoint`       | Stage-boundary workshop dump and artifact copy                                                                                      |
| `vbpl`             | VBPL gateway client (HTTP/1.1, rate and in-flight limits, size limits, bounded tree decoding)                                       |
| `filecache`        | Read-only local attachment cache keyed by document id, name and size                                                                |
| `source`           | Country source lists (`sources/<cc>.yaml`)                                                                                          |
| `store`, `migrate` | sqlc data layer and goose migrations for the workshop database                                                                      |
| `fault`            | Crash points compiled only with the `faultinject` build tag                                                                         |

Stages run in order `discover`, `fetch`, `extract`, `relate`, `chunk`, `embed`,
`build`, `eval`. A completed evaluation leaves the run `evaluated`.

## Local services

Native PostgreSQL 18 with pgvector (`workshop` and `corpus_template`) and the
Temporal development server (Temporal CLI 1.10.0, server 1.32.2) with SQLite
persistence, started in order by `make services-up`.

## Tests

`make test` (unit, about 2 s), `make test-integration` (PostgreSQL and a shared
Temporal dev server per package, about 5 s), `make test-proof` (crash, restart
and checkpoint recovery proofs, about 20 s), `make pre-push` (static checks). CI
runs them in `.github/workflows/ci.yml` and skips Go jobs for Markdown-only pull
requests.
