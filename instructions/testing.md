# Testing and evidence

|Layer|When|Command|
|-|-|-|
|Static|before every push|`make pre-push` ([checks](../scripts/pre-push.sh))|
|Unit|every push|`make test`|
|Integration with PostgreSQL|every push|`make test-integration` (native PostgreSQL from `make services-up`)|
|Corpus evaluation|before publishing a corpus|Use the staged `eval` report and signed upload gate in [the corpus release runbook](../docs/runbooks/release-restore.md#release-a-corpus); [ADR 0010](../docs/adr/0010-retrieval-evaluation-for-agents.md) owns the gates and targets.|
|End-to-end|pull requests when the changed paths require it|`make e2e` starts and tears down the local stack, running Go flow tests and Playwright. CI sets `SERVER_PUBLIC_SUMMARIES=true` for summary specs.|

Rules: write the failing test first; observe the failure before the fix. A report claims only checks that ran and shows failures with output. Reviewers read the primary's check output and the exact-commit CI run; they do not run checks. Never lower an evaluation gate or target, or edit golden cases, to admit a change; add a new case and record why. An ADR History entry may revise a gate or target when it records the measured evidence and the trade-off for agents; a change made only so a release passes is not such a revision.

- Unit tests never wait on real time beyond 100 ms; inject clocks, limiters and backoffs.
- Integration tests share one dev server per package and clone the template database.
- Proofs run under 60 seconds using the timing knobs.
- CI targets under 5 minutes for a pull request and skips Go jobs for Markdown-only changes.
- Integration tests and proofs share the local PostgreSQL with real release databases. Use test releases in 2099 from `internal/testpg` (never a real-looking release such as `20261109`), and drop a corpus database only through the marker-checked cleanup helper there; never `DROP DATABASE` by name alone.
