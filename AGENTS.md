# AGENTS.md

## Scope and authority

This file owns the shared rules for Claude Code and Codex in trachuan. `CLAUDE.md` imports it and adds Claude Code specifics. Component `AGENTS.md` files, added when a component exists, map local sources, boundaries and checks. Agent definitions in `.claude/agents/` and `.codex/agents/` are thin wrappers that name a duty and the instruction files that role reads.

Direct owner instructions and platform safety rules come first.

## What trachuan is

A citable corpus of Vietnamese law served to AI agents over MCP and to people through a website, behind Google login. Evidence only: no answer model, generative model or reranker runs on our servers; the query embedder is the only served model. Vietnam first; country, keys and routes carry a country code from day one. Successor to banhmi (archived); it reuses banhmi's data, never its code. Design: `docs/design/README.md` and `docs/adr/`.

Language: documents, code, comments and commits in English. Product interface in Vietnamese with optional English. Legal text is never translated.

## Map

|Question|Entry point|
|-|-|
|What should the system become?|`docs/design/README.md` and accepted ADRs in `docs/adr/`|
|Why was a choice made?|`docs/adr/` (one topic per file, with History)|
|What exists now?|Code, deployment configuration, `docs/architecture.md`|
|How is production operated?|`docs/runbooks/`|
|What work remains?|`docs/plans/`|
|How should documents read?|`docs/standards/engineering.md`|
|Where is old evidence?|`docs/research/`|

|Doing|Read first|
|-|-|
|Planning, briefing, implementing, reviewing|`instructions/workflow.md`|
|Go or SQL|`instructions/go.md`|
|Python (`apps/embedder`)|`instructions/python.md`|
|Nuxt or TypeScript|`instructions/web.md`|
|Containers, host, OpenTofu, releases|`instructions/deploy.md`|
|Tests, evaluation, CI evidence|`instructions/testing.md`|

Precedence: an accepted ADR wins over design text; design wins over a plan. Code, deployment configuration and the OpenAPI document must agree about implemented behavior; a disagreement is a defect to repair together.

## Agents

The owner talks to the primary Claude session, which runs as `manager`. It plans, writes briefs, verifies results, runs local checks, and owns Git, tags and deploys. Codex does design, design review, implementation and troubleshooting through `scripts/codex-agent.sh <role> <worktree> <brief> <report>`; Claude Opus does code review as a subagent.

|Role|Where|Model, effort|Duty|
|-|-|-|-|
|`designer`|Codex `.codex/agents/designer.toml`|`gpt-6-astra` high, workspace-write in `docs/`|Design notes, ADRs, contract proposals, UI specs; never code|
|`reviewer` (design)|Codex `.codex/agents/reviewer.toml`|`gpt-6-astra` high, read-only|Independent review of designs and ADRs before implementation|
|`implementer`|Codex `.codex/agents/implementer.toml`|`gpt-6.1-terra` high, workspace-write|One bounded task in named paths; root-causes and fixes a failure from supplied evidence|
|`reviewer` (code)|Claude `.claude/agents/reviewer.md`|Opus 5.5 xhigh, read-only|Independent code review and adversarial pass after the last code task|
|exploration|Claude `Explore` agent|default|Searches and mechanical reading|

No model reviews its own work. The primary states the route in every brief. Local builds and tests run on the owner's MacBook; the primary runs them and gives reviewers the output.

|Lane|Owned paths|Never|
|-|-|-|
|backend|`apps/server/`, `apps/corpus/`, `apps/embedder/`, root `internal/`, `packages/schema/`, migrations|Edits web UI or production infrastructure|
|frontend|`apps/web/`|Edits Go, migrations or infrastructure|
|devops|`deploy/`, `.github/workflows/`, release scripts|Reads secret values or applies infrastructure unreviewed|

## How we work

- Small releases, one change per patch tag. GitHub CI is the gate; a tag and a deploy need green CI on that exact commit.
- Write the failing test first, then the smallest correct change. Follow nearby code; do not add features, refactors or files beyond the brief.
- Keep files small: Markdown at most 300 lines (plans in `docs/plans/` exempt), non-test code at most 400 (`scripts/check-lengths.sh`, enforced in the pre-commit hook). Split by concern, one concern per file, named after it.
- Each fact has one home. Plans are deleted when done; design, ADRs, code and runbooks remain.
- Never commit secrets, personal data or user questions. The repository may become public.
- Plain words, active voice, no em dashes, no plan or task IDs in code or living docs. Agent-only files (this file, `instructions/`, `.claude/`, `.codex/`, `docs/plans/`) stay minified: one line per paragraph or list item.

## Briefs and reports

A brief names the objective, authorities to read, owned paths, forbidden actions, definition of done, checks the primary will run, and the report contract. Workers start with `git status --short` and leave existing changes alone. Stop at a document conflict, overlapping ownership or missing decision; report it instead of inventing a contract.

A report lists exact new, changed and deleted files; checks run with results; checks not run with the command and reason; evidence paths; open items. The Codex launcher saves only the final message, so the complete report goes there.

## Git

Only the primary changes Git state. Workers read status and diffs; they never add, commit, stash, reset, switch branches or create worktrees.
