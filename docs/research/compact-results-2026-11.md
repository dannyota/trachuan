# Compact search results: response size (2026-11-11)

Measured to check the size gate in
[search results](../design/mcp-results.md#evaluation). Not authority;
[ADR 0013](../adr/0013-compact-search-and-batch-read.md) decides.

## Setup

Corpus database `corpus_vn_20261110_4` (schema 3), the 92 golden questions one
at a time, hybrid recipe 1, evaluation date 2026-11-10, the Python INT8 query
service. Each response went through `Engine.Present` and was serialized with
`encoding/json` without whitespace; size is the rune count of the whole JSON
(documents, hits, snippets, paths, pointers, gaps; in full mode also text,
fragments, scores and relation evidence). A throwaway harness ran outside the
evaluation stage, because no committed build matched the branch's build recipe.

## Results

Size in code points per response:

| Detail, limit | Before shorter handles p50 / p95 | After p50 / p95   |
| ------------- | -------------------------------- | ----------------- |
| compact, 8    | 12,975 / 15,685                  | 11,880 / 14,345   |
| compact, 20   | 31,932 / 36,034                  | 29,155 / 33,186   |
| full, 8       | 58,062 / 80,305                  | 56,998 / 79,344   |
| full, 20      | 145,275 / 186,139                | 142,454 / 183,907 |

Composition of compact at 20 hits, before: snippets 32%, document key strings
206 per response (30%); after: snippets 34%, key strings 102 per response (16%).
A response names 11.0 documents with 4.5 pointers each on average; a hit has 2.9
path nodes.

## Reading

- The gate passes: compact at 20 hits is 33,186 at p95, well below full at 8
  hits (79,344), while carrying 2.5 times as many hits.
- Full serialized responses are about four times the evidence character count
  the evaluation report uses, mostly relation evidence and fragments.
- Repeated `vn:vbpl:<uuid>` keys were as large as the snippets; bare anchors
  inside one document and one pointer key cut them by half.

## Not measured

Token counts per client model, `read` call sizes, latency, the compact
diagnostic recall (the evaluation stage needs a committed build that matches the
branch's build recipe), and the agent-in-the-loop check, which needs the server.
