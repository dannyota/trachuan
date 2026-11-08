# Documentation

Each fact lives in the document type that owns it. Link to that source instead
of copying it.

| Question                                   | Authority                                     |
| ------------------------------------------ | --------------------------------------------- |
| What should the system become?             | [`design/`](design/README.md)                 |
| Why was a choice made?                     | [`adr/`](adr/README.md)                       |
| What does the repository implement now?    | Code, configuration, [`architecture.md`](architecture.md) |
| What runs next, and which gates apply?     | [`plans/`](plans/README.md)                   |
| How is an operation performed?             | [`runbooks/`](runbooks/README.md)             |
| How should documents and comments be written? | [`standards/engineering.md`](standards/engineering.md) |
| Where is historical evidence?              | [`research/`](research/README.md)             |

An accepted ADR wins over design text. Design wins over a plan. Code,
deployment configuration and the OpenAPI document must agree about implemented
behavior; a disagreement is a defect to repair together.

| Path              | Contents                                        | Lifecycle                                    |
| ----------------- | ----------------------------------------------- | -------------------------------------------- |
| `design/`         | Intended product and architecture               | Living; amended by accepted ADRs             |
| `adr/`            | One decision per topic, with History            | Updated with owner approval                  |
| `plans/`          | Remaining work and backlog                      | A plan is deleted when its work lands        |
| `runbooks/`       | Exact operating and recovery procedures         | Living with supported commands               |
| `research/`       | Observations, experiments, bake-off results     | Not authority                                |
| `standards/`      | Writing and comment rules                       | Living repository policy                     |

Start at [design](design/README.md) and the [ADR index](adr/README.md).
