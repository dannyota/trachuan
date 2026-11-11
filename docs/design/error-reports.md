# Agent error reports

Users' agents report a defect they found in the corpus: wrong text, wrong
validity, a broken citation or a missing document. Reports are triaged by the
operator and feed reviewed fixes; they never change the corpus by themselves.
Privacy: [ADR 0002](../adr/0002-evidence-only-vietnamese-queries.md) (questions
are never stored). Limits: [budgets](budgets.md). Table: [data](data.md).

## Report

A report is structured data only. It has no free-text field.

| Field        | Rule                                                                                                                                                                     |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `country`    | `vn`                                                                                                                                                                     |
| `kind`       | `wrong_text`, `wrong_validity`, `broken_citation` or `missing_document`                                                                                                  |
| `handle`     | A document or provision handle from [search results](mcp-results.md#handles); required except for `missing_document`, where it is the citing provision when there is one |
| `release`    | Optional; defaults to the active release; a retired release is `release_expired`                                                                                         |
| `suggestion` | Optional object whose allowed members depend on `kind` (below); every member typed and validated                                                                         |

Suggestion members by kind; any other member, or a member of another kind, is
`invalid_request`:

| Kind               | Members                                                                                                   |
| ------------------ | --------------------------------------------------------------------------------------------------------- |
| `wrong_text`       | `source_url`                                                                                              |
| `wrong_validity`   | `status_class` (the closed list of [corpus schema](corpus-schema.md)), `eff_from`, `eff_to`, `source_url` |
| `broken_citation`  | `document_number` (the intended target), `source_url`                                                     |
| `missing_document` | `document_number` (required for this kind), `issue_date`, `source_url`                                    |

Dates are ISO `YYYY-MM-DD` and `eff_from` is not after `eff_to`.
`document_number` is stored in the corpus normalized form ([data](data.md),
number normalization) and must match the document-number grammar the reference
parser accepts. `source_url` is `https://` on an allowlisted official host
(`vbpl.vn`, `vanban.chinhphu.vn`, `datafiles.chinhphu.vn`,
`congbao.chinhphu.vn`), at most 512 bytes, without user info, port or fragment.
So that no field can carry free text, its path holds only ASCII letters, digits
and `/._-` (no percent-encoding), and a query is allowed only on `vbpl.vn` with
`ItemID` and `dvid` holding digits. The server stores it and never fetches it.
The handle must resolve in the named release: an unknown document or anchor is
`invalid_request`.

## Interfaces

- MCP tool `report_issue` with the fields above. Result
  `{report_id, state, deduplicated}`; errors use the
  [search results](mcp-results.md) error shape. It shares the `read` per-account
  bucket and the report limits in budgets. `guide` gains one paragraph on when
  to report and that only structured fields are accepted.
- Web: `POST /api/v1/reports` (session and CSRF, same body and result),
  `GET /api/v1/me/reports` (own reports, newest first, at most 100) and
  `DELETE /api/v1/me/reports/{id}` (withdraw an own open report), with the
  [web API](web-api.md) rules for account routes.
- Admin CLI on the host: `server admin reports list|show|accept|reject|export`.

## Dedupe and limits

One open report per (account, document, anchor, kind): a repeat returns the
existing report with `deduplicated: true` and changes nothing, so retries are
safe. Reports of the same (document, anchor, kind) from different accounts are
separate rows; triage groups them and shows the number of distinct accounts,
which ranks the queue and resists one-account spam. Per-account daily and open
caps and a global open cap are in budgets; at a cap the call is `rate_limited`.
A blocked account cannot report.

## Triage

`reports list` shows groups by (document, anchor, kind) with the report count,
the distinct-account count, the newest time and the suggestions' distinct
values; it never shows account identities. `reports show <group>` adds report
ids and states; account ids appear only with `--accounts`, and email or Google
subject never. The CLI runs only on the host with the auth database role; there
is no web triage and no user ever sees another account's reports.

`accept` and `reject` act on a group or on report ids, set `decided_at` and
state, and record the operator name given on the command line. A rejected or
withdrawn report closes; accepting does not apply anything.

## Export

`reports export --out <dir>` writes reviewable drafts of accepted reports
without account data:

- `corrections-draft.yaml`: one entry per accepted `wrong_validity` group in the
  format of `sources/vn-corrections.yaml` (key, number, `set` from the agreed
  suggestion values), with `reason` and `evidence` left as `TODO` and
  `reviewed: ""`; it is never merged into that file automatically;
- `text-issues.jsonl`: accepted `wrong_text` groups for extraction and OCR
  review;
- `citation-review.jsonl`: accepted `broken_citation` and `missing_document`
  groups for identity-map review and source discovery.

A person reviews each draft against official text before copying it into the
sources or the identity map; the export marks the exported groups so the next
export lists only new ones.

## Retention

Open reports stay until decided or withdrawn. Rejected, withdrawn and duplicate
reports are deleted after the period in budgets; accepted reports after their
export plus the same period. Account deletion deletes the account's reports
(exports carry no account data). The purge runs in the server's hourly loop.

## Alternatives rejected

- A free-text note. Pattern rules cannot reliably keep a user's question or
  personal details out of free text, and ADR 0002 forbids storing questions;
  every useful hint has a typed field instead.
- Anonymous reports: no abuse control or dedupe.
- Applying corrections automatically: every fix needs reviewed evidence.
- Storing reports in the release database: release databases are immutable and
  replaced per release.
