# ADR 0013: Compact search results and batch reading for agents

Status: accepted 2026-11-10. The owner approved the direction and delegated the
details with the rule "we return results to agents, and you are agent".

## Context

The main consumers are AI agents over MCP: Claude Sonnet and Opus, and OpenAI
terra and sol models. An agent reads every hit it receives, pays for each hit in
context tokens, and often searches several times per task
([ADR 0010](0010-retrieval-evaluation-for-agents.md)). The first MCP contract in
[retrieval](../design/retrieval.md) returned up to 14,000 code points of
verbatim text per hit, with relation evidence, and a separate `document` tool
for one target per call. On the baseline release, 8 hits measured 13,520
evidence characters at p50 and 24,531 at p95; 20 hits measured 37,978 and
54,027, while recall rose from 0.677 to 0.742
([baseline note](../research/index-baseline-2026-11.md)).

An agent decides what to read from cheap signals first: which document, which
article and clause, whether the document is in force, whether another document
amends, replaces or implements it, and a short passage showing why the hit
matched. Relations in the corpus are resolved at document level today: none of
the 14,311 relations in `vn/20261110_2` names a target provision, and 6,675 name
a target document in the corpus.

## Decision

1. `search` returns compact hits by default: document metadata once per
   document, then ranked hits, each with its citation path, a verbatim snippet
   with a completeness flag, and handles. `detail:"full"` returns the recipe
   evidence of the first contract instead. Ranking, abstention and validity are
   recipe 1, unchanged.
2. Document-level relations appear on each returned document as counts by
   direction and kind; full results and `read` also list them as bounded
   pointers. Provision-level relations stay on the hit as warnings.
3. `read` replaces `document`. One call reads several handles: a document handle
   returns its metadata and outline, a provision handle returns its verbatim
   text with its ancestors' lead-ins, and a relation item lists every counted
   relation. A per-call cap applies; parts are fixed per release, every call
   makes progress, and opaque `next` tokens say what to read next.
4. The compact default limit is 20 hits; the full default stays 8; the maximum
   stays 50. Numbers live in [budgets](../design/budgets.md).
5. Results are JSON in `structuredContent` with the same compact JSON in one
   text block, as the MCP tools specification recommends.
6. `guide` teaches the flow search, scan, read chosen handles in one call, check
   validity and relations, and cite only complete text that was read.

The [search results design](../design/mcp-results.md) owns the shapes, the
snippet rule and the read rules.

## Consequences

Typical searches cost a fraction of the old evidence, so an agent can run more
searches and see more candidates per task; a second call is needed before citing
a long provision. ADR 0010 product scoring and promotion keep running on full
responses at limits 8 and 20; compact responses get the same matcher as a
diagnostic, without the child or relation credit they cannot carry. Evaluation
reports response size for both modes. Before release 1, an agent-in-the-loop
check with one Claude and one OpenAI client model compares compact and full on
development cases for cited expected provisions, tool calls and total input
tokens. Compact stays the default only if, for each model, cited provisions do
not fall and tokens are lower.

A snippet is not the provision. A snippet that cuts text is marked incomplete
and shows each omission with the marker `[…]`, the only text not taken from the
source. Relation pointers show the stored source-to-target direction and kind;
they never state an inverse legal effect.

## Alternatives rejected

- Full text by default with added pointers: one call per answer, but 8 hits
  already reach 24,531 characters at p95, and 20 hits 54,027, which crowds out
  further searches.
- Compact only, without `detail:"full"`: forces two calls for short tasks and
  removes the mode the evaluation history was measured in.
- A separate tool per reading mode (outline, provision, relations): more tools
  to choose from; one `read` with handles covers them.
- Generated summaries of provisions: a platform model, rejected by
  [ADR 0002](0002-evidence-only-vietnamese-queries.md).

## History

- 2026-11-10: proposed after the owner asked for results that let agent models
  choose what to read, including related parts inside and across documents.
- 2026-11-10: independent review (three rounds, findings S-01 to S-09) fixed
  completeness, continuation, relation reach, endpoint identity, snippet
  determinism and evaluation; verdict merge.
- 2026-11-10: accepted by the owner.
- 2026-11-11: measured on vn/20261110_4 (92 golden questions): compact at 20
  hits is 31,932 code points at p50 and 36,034 at p95, full at 8 is 58,062 and
  80,305. Document keys were 30% of compact size, so positions inside one
  document became bare anchors and pointers dropped the duplicate handle.
- 2026-11-14: item 2 revised: compact document entries keep `related_counts` and
  drop the `related` pointers, which were 9 of 22 KB of document entries in a 45
  KB compact result; full results and `read` keep them. It ships only under the
  gate in [search results](../design/mcp-results.md#evaluation): against current
  compact, per model, cited expected provisions do not fall and summed input
  tokens are lower; the guide reads relations when counts warrant.
