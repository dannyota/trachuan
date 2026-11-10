# Workflow

The primary session (`manager`) plans, writes and reviews designs, ADRs and documentation, makes the decisions, briefs, verifies and owns Git. Codex workers do implementation and troubleshooting; Codex `gpt-6-astra` reviews architecture designs and code independently. The primary asks the owner only for decisions the owner must make, one at a time, with a recommendation.

## Routes

|Work|Role and model|
|-|-|
|Design, ADR, contract proposal, documentation, decisions|The primary (`manager`), which reviews its own documentation and prose|
|Independent review of an architecture design or ADR|Codex `reviewer`, `gpt-6-astra` high (`--search` when sources are needed)|
|Implementation, troubleshooting with supplied evidence|Codex `implementer`, `gpt-6.1-terra` high|
|Review of code, adversarial pass|Codex `reviewer`, `gpt-6-astra` high|
|Searches, summaries, mechanical reading|Claude `Explore` agent|

Dispatch: `scripts/codex-agent.sh [--model <m>] [--effort <e>] [--search] [--resume <session-id>] <role> <worktree> <brief-file> <report-file>`. The primary decides and states the route in each brief. No model reviews its own work; the original reviewer confirms each fix.

## Flow

1. Read the design and accepted ADRs for the area. An accepted ADR wins over design text; design wins over a plan.
2. For a contract or architecture change, the primary writes the proposal and the Codex `reviewer` reviews it before any code. Documentation and prose the primary wrote need no independent review. Small changes skip design.
3. Brief one implementer per bounded task with exact owned paths. Failing test first, then the smallest fix. Adversarial cases for hostile input, authorization, races and bounds belong to the author.
4. The primary runs `make pre-push` and the checks the brief names, inspects the diff and report, then dispatches the Codex `reviewer` once after the last code task. One-line and test-only changes skip review.
5. Return findings to the author; the same reviewer confirms the fixes.
6. Push a branch, open a draft pull request, mark it ready when complete. Landing needs independent review and green CI on the exact head. The owner merges with a signed merge; delete merged branches.
7. When CI fails, the primary saves the failing job logs to the scratchpad and briefs `implementer` with them. Two red runs from the same cause means stop and repair the process.

## Reviews

Security-relevant work beyond one line needs an adversarial pass before merge: login, sessions, OAuth, tokens, CIMD fetching, rate limits, corpus release activation, backups, deploy and release workflows. Model these attackers: an anonymous visitor; a signed-in user attacking another account; a hostile MCP client or CIMD document; a stolen token or cookie; a network position between client and edge; a compromised container on the host; a leaked backup.

Review output: Verdict (`merge`, `fix first`, `redesign`); Findings ordered blocker, major, minor as `R-nn. [severity] [category] file:line` for code, `S-nn.` for design, `A-nn. [attacker]` for adversarial; Checks read; Not checked. Quote the text, state the failure scenario, give one fix sentence. Do not write the fix.

## Briefs and reports

Follow AGENTS.md "Briefs and reports". Workers report an edit needed outside their paths instead of making it. Never hand-edit generated output; regenerate from the source and report every generated file changed.
