# Discovery Plan (Plan 18)

> **For agentic workers:** Routes (`instructions/workflow.md`): D1 and D2 are routine web work by Codex `implementer` (`gpt-6.1-terra`) in `apps/web/`, reviewed by `reviewer` (`gpt-6.1-sol`); the codes endpoint is backend work briefed to a dev session. The primary runs `make web-test`, `make web-build`, `make pre-push` and, for the backend PR, `make test` and `make test-integration`.

**Goal:** search engines and AI answer engines find and quote Tra Chuẩn's public pages as [discovery](../design/discovery.md) describes. The spec wins over this plan.

**Staging is authorized:** the design describes the target state. Each stage below implements the part it names and leaves the rest as it is today; a stage never contradicts the spec in what it does build. D1 keeps today's titles and descriptions (D2 changes them) and lists only the six fixed URLs in the sitemap (topic codes wait for the codes endpoint).

## Stages

| Stage | Contents | Depends on | State |
|-|-|-|-|
| D1, web | robots.txt; sitemap.xml with the six fixed URLs and `lastmod` on privacy and terms from one ISO date; a `usePublicSeo` composable giving every indexable page its canonical, Open Graph and Twitter tags with today's titles and descriptions; the fixture-API sweep; tests | Plan 17 stage 1 | done |
| D2, web | the titles and descriptions of discovery.md; the `/sources` counts-and-date sentence; JSON-LD for home, topics and sources (the Person node and the Organization `founder` once the maker panel is on home, which it is); llms.txt in the standard shape; the CSP test widening; `tests/public-metadata.test.ts` | D1 | in progress |
| Topic codes, backend and web | `GET /api/v1/public/taxonomy/codes` with its OpenAPI contract, generated client and Go contract tests; the sitemap reads it under the completeness rule | D1 | planned |
| D3 | public document summaries; separate design and review (gpt-6-astra) | astra acceptance | design |

## Owner actions

- After D1 is deployed: a Search Console Domain property by DNS TXT at Cloudflare; the Bing import; submit `/sitemap.xml` in both.
