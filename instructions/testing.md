# Testing and evidence

|Layer|When|Command|
|-|-|-|
|Static|before every push|`make pre-push`: gofmt, goimports, golangci-lint, eslint, prettier check, markdownlint, lengths, shellcheck|
|Unit|every push|`make test`|
|Integration with PostgreSQL|every push|`make test-integration` (compose database)|
|Corpus evaluation|before every `corpus release`|`make eval`: golden set floors recall@8 0.90, MRR@8 0.66, current-law precision 0.99, abstention 0.95; filtered cases; diff against previous release|
|Browser|every pull request|`make e2e`: Playwright against the local stack with the fake Google provider|

Rules: write the failing test first; observe the failure before the fix. A report claims only checks that ran and shows failures with output. Reviewers read the primary's check output and the exact-commit CI run; they do not run checks. Never lower an evaluation floor or edit golden cases to admit a change; add a new case and record why.
