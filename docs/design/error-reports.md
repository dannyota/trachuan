# Agent error reports

Users' agents report a defect they found in the corpus: wrong text, wrong
validity, a broken citation or a missing document. Reports are triaged by the
operator and feed reviewed fixes; they never change the corpus by themselves.
Privacy: [ADR 0002](../adr/0002-evidence-only-vietnamese-queries.md) (questions
are never stored). Limits: [budgets](budgets.md). Table: [data](data.md).

## Report

A report is structured data only. It has no free-text field and no URL.

| Field        | Rule                                                                                                                                                          |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `country`    | `vn`                                                                                                                                                          |
| `kind`       | `wrong_text`, `wrong_validity`, `broken_citation` or `missing_document`                                                                                       |
| `handle`     | A document or provision handle from [search results](mcp-results.md#handles); required except for `missing_document`, where it is the citing provision if any |
| `release`    | Optional; defaults to the active release; a release that is not active is `release_expired`                                                                   |
| `suggestion` | Optional object whose allowed members depend on `kind` (below)                                                                                                |

| Kind               | Suggestion members                                                                          |
| ------------------ | ------------------------------------------------------------------------------------------- |
| `wrong_text`       | none                                                                                        |
| `wrong_validity`   | `status_class` (the closed list of [corpus schema](corpus-schema.md)), `eff_from`, `eff_to` |
| `broken_citation`  | `document_number`: the intended target, which must be a document of the release             |
| `missing_document` | `document_number` (required), `issue_date`                                                  |

Any other member, or a member of another kind, is `invalid_request`. Dates are
ISO `YYYY-MM-DD`: `issue_date` between 1945-09-02 and today, `eff_from` and
`eff_to` between 1945-09-02 and ten years after today (a `not_yet` document
starts in the future), and `eff_from` is not after `eff_to`. A `document_number`
is normalized as the corpus `number_norm` ([data](data.md)). For
`broken_citation` it must equal the `number_norm` of a document of the release;
no grammar applies, so older numbers without a slash (such as `196-HĐBT`) are
accepted. For `missing_document` it must have the form `<1 to 4 digits>/<year>/`
followed by one to four segments, each of which already appears as a segment of
some document number in the release (document types and issuers), and the year
lies between 1945 and the current year. The handle must resolve in the release:
an unknown document or anchor is `invalid_request`.

These rules leave no field where a sentence fits. A client that wants to leak
information can still choose handles, kinds and values to encode a few bits per
report; the per-account and global caps bound that channel, and the operator
sees only corpus identifiers and closed values.

## Stored identity

A report row keeps the corpus identity it was checked against: `release`,
`document_key`, `anchor` (empty for a whole document or a `missing_document`
without handle), the document's `document_number` and the normalized `target`
number (`broken_citation` and `missing_document`; empty otherwise), so triage
and export work after the release is retired. The dedupe and group key is
(document_key, anchor, kind, target), the same across releases because document
keys and anchors are stable; the report keeps the release it was made against.

## Interfaces

- MCP tool `report_issue` with the fields above, annotated
  `readOnlyHint: false`, `destructiveHint: false`, `idempotentHint: true`,
  `openWorldHint: false`. Result `{report_id, state, deduplicated}`; errors use
  the [search results](mcp-results.md) error shape. It shares the `read`
  per-account bucket. `guide` gains one paragraph on when to report and that
  only structured fields are accepted.
- Web: `POST /api/v1/reports` (session and CSRF, same body and result),
  `GET /api/v1/me/reports` (own reports, newest first, pages of 100 with a
  `next` cursor) and `DELETE /api/v1/me/reports/{id}` (withdraw an own open
  report), with the [web API](web-api.md) rules for account routes.
- Admin CLI on the host: `server admin reports list|show|accept|reject|export`.

## Admission and limits

One open report per (account, group key): a repeat returns the existing report
with `deduplicated: true` and changes nothing. Admission runs in one
transaction: it locks the account row (rejecting a blocked or deletion-requested
account), takes the reports advisory lock for the global counts, then checks the
per-account daily, open and retained caps and the global retained cap in budgets
before inserting; at a cap the call is `rate_limited`. The global retained cap
counts report rows and export snapshot rows together, and batch creation takes
the same lock and is refused at the cap with a fixed message. Withdrawn and
closed reports keep counting against the retained caps until deleted, so
submitting and withdrawing cannot exceed them.

## Triage

`reports list` shows groups by group key, each with a group id (the SHA-256 of
the key, shown as its first 12 hexadecimal characters), the report count, the
distinct-account count, the newest time and the distinct suggestion values; it
never shows account identities. `reports show <group id>` adds report ids,
releases and states; account ids appear only with `--accounts`, and email or
Google subject never. The CLI runs only on the host with the auth database role;
there is no web triage and no user sees another account's reports. `show`,
`accept` and `reject` take a group id or a prefix of at least 12 characters that
matches exactly one group, or report ids; an ambiguous or unknown selector is
refused without any change. `accept` and `reject` act and record the operator
name given on the command line. Accepting applies nothing.

## Export

`reports export --out DIR` creates a batch in one transaction: it assigns a new
batch id to every accepted report not yet in a batch and copies each one's group
key, document number, release, kind, target and suggestion, without account
data, into `issue_report_export` ([data](data.md)). That snapshot is the batch's
fixed membership: later account deletion or report retention does not change it.
The files are written from the snapshot into a fresh temporary directory beside
`DIR/<batch id>/`, with a manifest of report ids, and the directory is renamed
into place once complete; an existing `DIR/<batch id>/` is refused.
`reports export --batch ID --out DIR` writes the same batch again from its
snapshot into a fresh directory, so a crash at any point is repaired by
rerunning it; a batch whose snapshot has been deleted by retention fails with a
fixed message. The files are:

- `corrections-draft.yaml`: one entry per `wrong_validity` group whose anchor is
  empty and whose accepted reports agree on one non-empty set of suggestion
  values, in the format of `sources/vn-corrections.yaml` (key, number, `set`),
  with `reason` and `evidence` left as `TODO` and `reviewed: ""`;
- `validity-review.jsonl`: every other accepted `wrong_validity` group
  (provision-level, conflicting or empty suggestions), never a draft;
- `text-issues.jsonl`: accepted `wrong_text` groups for extraction and OCR
  review;
- `citation-review.jsonl`: accepted `broken_citation` and `missing_document`
  groups for identity-map review and source discovery.

A person reviews each draft against official text before copying it into the
sources or the identity map; nothing is applied automatically.

## Retention

Open reports stay until decided or withdrawn. Every other report is deleted once
the period in budgets has passed since its last state change (accepted,
rejected, withdrawn or exported), so accepted reports must be exported within
that period; batch snapshots are deleted after the same period from their
creation. Account deletion deletes the account's reports; batches already
exported carry no account data. The purge runs in the server's hourly loop.

## Alternatives rejected

- A free-text note. Pattern rules cannot reliably keep a user's question or
  personal details out of free text, and ADR 0002 forbids storing questions;
  every useful hint has a typed field instead.
- A source URL. Its path and query can carry text, and the reviewer finds the
  official source from the document anyway.
- Anonymous reports: no abuse control or dedupe.
- Applying corrections automatically: every fix needs reviewed evidence.
- Storing reports in the release database: release databases are immutable and
  replaced per release.
