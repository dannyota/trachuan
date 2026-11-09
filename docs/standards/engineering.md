# Engineering documentation and comments

Write each fact once, in the artifact that owns it, in short plain words. Plans
say what to do next and are deleted when the work lands; design docs and ADRs
say how the system is and why. Code, tests and living docs must stand on their
own after every plan is gone. Do not use em dashes.

## Ownership

| Information                            | Owner                                                |
| -------------------------------------- | ---------------------------------------------------- |
| Intended product or architecture       | `docs/design/`                                       |
| Numeric limits                         | `docs/design/budgets.md`                             |
| One accepted choice and its trade-offs | `docs/adr/`                                          |
| Current implemented system             | Code, configuration, OpenAPI, `docs/architecture.md` |
| Work order and delivery state          | `docs/plans/`                                        |
| Operational procedure                  | `docs/runbooks/`                                     |
| Contributor guidance                   | Repository or component `README.md`                  |

## Writing

- Lead with the result. One idea per sentence. Active voice. Name the exact
  file, command or value instead of "it" or "some".
- Markdown that people read wraps at 80 columns (Prettier). Agent-only files
  stay minified: one line per paragraph or list item.
- Markdown files are at most 300 lines; split by topic. Plans under
  `docs/plans/` are exempt because they are deleted when their work lands.
  Non-test code files are at most 400 lines; split by concern.
  `scripts/check-lengths.sh` enforces both.
- Comments explain why, not what. A comment that restates the code is deleted.
- Verify before asserting: cite first-party documentation for product behavior
  and mark what stays unverified.

## Naming

| Item          | Convention                                                  |
| ------------- | ----------------------------------------------------------- |
| Go file       | `<concern>.go`, `<subject>[_<scenario>]_test.go`            |
| Go test       | `TestSubject_Behavior` with meaningful subtests             |
| Vitest        | `<subject>.test.ts`                                         |
| Playwright    | `<feature>[-<scenario>].spec.ts`                            |
| Shell test    | `<subject>-test.sh`                                         |
| Workflow file | lowercase kebab case `.yml`; stable job IDs; aggregate `ci` |
| ADR           | `NNNN-<topic>.md`, sentence-case title                      |

## ADR rules

A record has Status, Context, Decision, Consequences, Alternatives rejected and
History. Changing a decision in force means updating its record with the owner's
approval and a History line. A new record is for a new topic only.
