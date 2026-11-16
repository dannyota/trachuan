# Discovery

How search engines and AI answer engines find, index and quote Tra Chuẩn. Routes
and caching: [web](web.md). Voice for every title and description:
[web voice](web-voice.md).

## Indexable routes

The six fixed URLs `/`, `/vn/topics`, `/connect`, `/sources`, `/privacy` and
`/terms`, `/vn/topics/{code}` for every served code, and, while document
summaries are available, every `/vn/documents/{key}/summary` whose `index` is
true, are indexable. Nothing else is.

| Route                                                               | Index                                                | Sitemap | How it stays out                                                                         |
| ------------------------------------------------------------------- | ---------------------------------------------------- | ------- | ---------------------------------------------------------------------------------------- |
| the six fixed URLs, served topic codes                              | yes                                                  | yes     |                                                                                          |
| `/llms.txt`, `/robots.txt`, `/sitemap.xml`                          | no                                                   | no      | not pages                                                                                |
| `/vn/documents/{key}/summary`                                       | when its `index` is true and summaries are available | same    | `X-Robots-Tag: noindex` otherwise ([public document summaries](web-public-documents.md)) |
| `/vn/search`, `/vn/documents/{key}`                                 | no                                                   | no      | 302 to `/login` without a session                                                        |
| `/login`, `/login/factor`, `/signup`, `/reset`, `/link`, `/account` | no                                                   | no      | `X-Robots-Tag: noindex`                                                                  |
| fixed 404, unavailable page                                         | no                                                   | no      | status 404 or 503                                                                        |
| `/api/`, `/mcp`, `/oauth/`                                          | no                                                   | no      | disallowed in robots.txt; not HTML pages                                                 |

Three separate mechanisms keep private content out. Protection: search results
and document text need a session, so an anonymous request never receives them (a
302 for pages, a 401 from the API). Redirect: a crawler that follows the 302
lands on `/login`
([Google: HTTP status codes](https://developers.google.com/crawling/docs/troubleshooting/http-status-codes)).
Noindex on the destination: the account pages answer `X-Robots-Tag: noindex`,
which works only when the crawler can fetch them
([Google](https://developers.google.com/search/docs/crawling-indexing/block-indexing),
[Bing](https://www.bing.com/webmasters/help/block-urls-from-bing-264e560a)). So
no HTML page route is disallowed in robots.txt, and no robots header is added to
a 302.

## robots.txt

Served as `text/plain` from a Nitro route, cached like `/llms.txt`, with the
origin from `runtimeConfig.public.siteOrigin`:

```text
User-agent: *
Allow: /
Disallow: /api/
Disallow: /mcp
Disallow: /oauth/
Sitemap: https://trachuan.vn/sitemap.xml
```

No group names a crawler. The wildcard admits search crawlers (Googlebot,
Bingbot, OAI-SearchBot, Claude-SearchBot, PerplexityBot) and training crawlers
(GPTBot, ClaudeBot) and leaves the Google-Extended token allowed, which keeps
Tra Chuẩn eligible for grounding in Gemini Apps. Among user-triggered fetchers,
ChatGPT-User may not apply robots rules, Perplexity-User generally ignores them,
and Claude-User follows a disallow
([OpenAI](https://developers.openai.com/api/docs/bots),
[Anthropic](https://support.claude.com/en/articles/8896518-does-anthropic-crawl-data-from-the-web-and-how-can-site-owners-block-the-crawler),
[Google](https://developers.google.com/crawling/docs/crawlers-fetchers/google-common-crawlers#google-extended),
[Perplexity](https://docs.perplexity.ai/docs/resources/perplexity-crawlers)).
The owner allows AI training (2026-11-16); an opt-out would name GPTBot,
ClaudeBot and Google-Extended, and refusing Google-Extended also removes Gemini
grounding.

## Sitemap

`/sitemap.xml` is a Nitro route in the exact-match route table, cached like the
other public routes. It lists the six fixed URLs and, from
`GET /api/v1/public/taxonomy/codes`, every served topic code. That endpoint
answers `{release, codes}` with the complete served tree of the active release
in one response, or 503. The sitemap is served only when it is complete: a
failed codes request, or a `release` different from `/api/v1/public/status`'s,
answers an uncached 503, never a partial file. The existing taxonomy routes
cannot list every code: an answer can be truncated at 500 nodes without saying
which were left out.

While document summaries are available, `/sitemap.xml` becomes an index over a
pages child (the URLs above) and a documents child of every summary with `index`
true ([public document summaries](web-public-documents.md)).

`lastmod` appears only on `/privacy` and `/terms`, from the same ISO date that
the pages show as "last updated". Every other URL omits it, because Google uses
`lastmod` only when it is consistently accurate
([build a sitemap](https://developers.google.com/search/docs/crawling-indexing/sitemaps/build-sitemap)),
and `/` and `/sources` change with copy, the stored sample and structured data
as well as with the corpus.

## Head tags

Every indexable page has, in the rendered language:

- `<title>` and `meta name=description`, the title at most 60 characters and the
  description at most 160 after NFC;
- `link rel=canonical`: `https://trachuan.vn` plus the path, no query, no
  trailing slash;
- `og:type` website, `og:site_name` Tra Chuẩn, `og:title`, `og:description`,
  `og:url` equal to the canonical, `og:locale` `vi_VN` or `en_US`;
- `og:image` `https://trachuan.vn/og-image.png` with `og:image:width` 1200,
  `og:image:height` 630 and `og:image:alt`;
- `twitter:card` summary_large_image.

Every other page that Nuxt renders, search and the reader included, carries a
site-level fallback, so a shared link still shows a preview: `og:site_name` Tra
Chuẩn, `og:title` "Tra Chuẩn", the same `og:image` with its size and alt,
`twitter:card`, and `og:locale` from the rendered language. It has no
description, no `og:description`, no canonical and no `og:url`. Indexable pages
override the fallback with their own tags. The static 404 and unavailable
documents carry no preview tags.

| Path                | Title vi                                          | Description vi                                                                                                                                    |
| ------------------- | ------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `/`                 | Tra Chuẩn · Tra cứu nguyên văn pháp luật Việt Nam | Hỏi bằng tiếng Việt, đọc nguyên văn điều, khoản trong văn bản pháp luật Việt Nam, kèm số hiệu và trạng thái hiệu lực. Dùng trên web hoặc qua MCP. |
| `/vn/topics`        | Chủ đề pháp luật theo Bộ pháp điển · Tra Chuẩn    | Duyệt văn bản pháp luật Việt Nam theo cây chủ đề của Bộ pháp điển, kèm số văn bản và số quy định trong từng chủ đề.                               |
| `/vn/topics/{code}` | {tên chủ đề} · Chủ đề · Tra Chuẩn                 | the node's description, else "Văn bản và chủ đề con của “{tên}” trong Bộ pháp điển: {n} văn bản, {m} quy định."                                   |
| `/connect`          | Kết nối trợ lý AI qua MCP · Tra Chuẩn             | Cách kết nối Claude, ChatGPT, Claude Code và Cursor với Tra Chuẩn qua MCP để tra cứu và trích nguyên văn pháp luật Việt Nam.                      |
| `/sources`          | Nguồn dữ liệu và phạm vi · Tra Chuẩn              | Văn bản lấy từ Cơ sở dữ liệu quốc gia về văn bản pháp luật và Cổng thông tin điện tử Chính phủ; bản phát hành hiện tại và những gì còn thiếu.     |
| `/privacy`          | Quyền riêng tư · Tra Chuẩn                        | Tra Chuẩn lưu gì và không lưu gì: tài khoản, câu hỏi, nhật ký, thời gian lưu và các bên xử lý dữ liệu.                                            |
| `/terms`            | Điều khoản sử dụng · Tra Chuẩn                    | Tra Chuẩn đưa bằng chứng từ văn bản pháp luật, không phải tư vấn pháp lý. Giới hạn sử dụng, tạm ngưng và xóa tài khoản.                           |

English, in the same order: "Tra Chuẩn · Vietnamese law, word for word" / "Ask
in Vietnamese and read articles and clauses of Vietnamese legal documents word
for word, with document numbers and status. On the web or over MCP."; "Legal
topics from the legal code · Tra Chuẩn" / "Browse Vietnamese legal documents by
the topic tree of the legal code, with the number of documents and provisions in
each topic."; "{topic name} · Topics · Tra Chuẩn" / the description, else
"Documents and subtopics under “{name}” in the legal code: {n} documents, {m}
provisions."; "Connect AI assistants over MCP · Tra Chuẩn" / "How to connect
Claude, ChatGPT, Claude Code and Cursor to Tra Chuẩn over MCP to search and
quote Vietnamese law word for word."; "Sources and coverage · Tra Chuẩn" /
"Documents come from the national legal database and the Government Portal. See
the current release, its counts and what is still missing."; "Privacy · Tra
Chuẩn" / "What Tra Chuẩn stores and does not store: accounts, questions, logs,
how long data is kept and who processes it."; "Terms of use · Tra Chuẩn" / "Tra
Chuẩn gives evidence from legal texts, not legal advice. Usage limits,
suspension and account deletion."

A topic name is cut on a word boundary so its title stays under 60, and a node
description under 160; counts come from the taxonomy response. Topic names stay
Vietnamese in both languages.

## English

The language is the `lang` cookie on the same URL, so only Vietnamese is
indexed. This is a deliberate limit: English URLs with `hreflang`, which Google
recommends over cookies
([multilingual sites](https://developers.google.com/search/docs/specialty/international/managing-multi-regional-sites)),
would double the route table, cache entries and head-tag tests, while topic
names, document titles and legal text stay Vietnamese on every page. There is no
`og:locale:alternate`.

## Structured data

JSON-LD is a `<script type="application/ld+json">` data block, which the browser
never runs, so `script-src 'self'` does not apply
([HTML scripting](https://html.spec.whatwg.org/multipage/scripting.html)). It is
emitted through `useHead` from `JSON.stringify(graph)` with every `<` replaced
by the six characters `\u003c`, because a data block still ends at the first
`</script`; titles and names come from data. The CSP test allows inline scripts
only as `application/json` or `application/ld+json`. Every node describes
content visible on its page
([Google structured-data policies](https://developers.google.com/search/docs/appearance/structured-data/sd-policies)),
and no node states legal validity: no `legislationLegalForce`, no status.

| Page                | Nodes                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| ------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `/`                 | `WebSite` (name, url, `inLanguage` vi, publisher the Organization); `Organization` (`@id` `https://trachuan.vn/#organization`, name, url, logo `/icon-512.png`, email `support@trachuan.vn`, founder the Person); `WebApplication` (name, url, `applicationCategory` ReferenceApplication, `operatingSystem` Web) with its own description, which says Tra Chuẩn does not write answers and does not give legal advice; `Person` (Danny, url and `sameAs` `https://aboutme.vn/danny`, image the maker picture) |
| `/vn/topics/{code}` | `BreadcrumbList` of the node path; `ItemList` of the node's main documents, each a `Legislation` with name, `legislationIdentifier`, `legislationType` and `legislationJurisdiction` VN, without url                                                                                                                                                                                                                                                                                                           |
| `/sources`          | `Dataset`: name, a description of 50 to 5,000 characters stating the sources and the release, document, provision and topic counts from `/api/v1/public/status`, url, `version` the release id, `dateModified` the release date, `inLanguage` vi, `spatialCoverage` Việt Nam, `creator` and `publisher` the Organization, `isBasedOn` the two sources; no `license` until the owner defines what it would cover                                                                                                |

## Facts an answer engine can quote

Each fact is one plain, server-rendered sentence that stands alone, with numbers
next to their unit and date.

| Fact                                                                                                    | Where                                                                                                                                                                                                         |
| ------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| What it is: finds passages and shows them word for word, with number and status; does not write answers | home lead; llms.txt summary; WebApplication description                                                                                                                                                       |
| Who makes it: Danny, linked to aboutme.vn/danny                                                         | home maker panel; Person node; llms.txt                                                                                                                                                                       |
| What it covers, and how current                                                                         | home scope line; on `/sources`, "Bản phát hành {release} có {documents} văn bản và {provisions} quy định; nguồn mới nhất ngày {date}.", without the clause after the semicolon when the newest date is absent |
| What it does not cover                                                                                  | the `/sources` gap list                                                                                                                                                                                       |
| How to connect by MCP                                                                                   | `/connect`; llms.txt                                                                                                                                                                                          |
| Evidence, not legal advice                                                                              | `/terms`; llms.txt summary; WebApplication description                                                                                                                                                        |

`/llms.txt` follows the standard shape ([llmstxt.org](https://llmstxt.org/)): an
H1, a blockquote summary that opens with today's first sentence and says that
Tra Chuẩn does not write answers or give legal advice and is made by Danny
(`https://aboutme.vn/danny`), the endpoint line and the two guidance paragraphs
verbatim, then a "Pages" section of absolute links to connect, sources, topics,
privacy and terms.

## Measurement

The owner verifies a Search Console Domain property by a DNS TXT record at
Cloudflare that stays in place
([Search Console](https://support.google.com/webmasters/answer/9008080)),
imports the site into Bing Webmaster Tools
([Bing](https://www.bing.com/webmasters/help/add-and-verify-site-12184f8b)), and
submits the sitemap. No third-party script runs. Monthly: page indexing (fixed
URLs, topics and summaries with `index` true indexed, other summaries and the
account pages excluded by noindex, search and reader URLs not indexed, nothing
"indexed, though blocked by robots.txt"), sitemap processing, queries for the
brand, topic names and document numbers, and whether answer engines cite
trachuan.vn, checked by hand.

## Tests

- robots.txt: exact body, content type, cache headers.
- sitemap.xml: well-formed; exactly the expected URLs; `lastmod` only on privacy
  and terms, equal to the pages' date; uncached 503 on a failed codes request or
  a release mismatch.
- Head tags, for every indexable route in both languages: one canonical,
  absolute, https, no query; `og:url` equal to it; `og:locale` matching the
  language; title and description within their limits after NFC; `og:image`
  absolute with width, height and alt. Every other page Nuxt renders carries the
  site-level fallback and no description, canonical or `og:url`; the static 404
  and unavailable documents are unchanged.
- JSON-LD: parses; the expected types; no legal-force or status property; a
  fixture title containing `</script><script>alert(1)</script>` stays inside one
  block in the server-rendered response and parses back unchanged.
- llms.txt: H1, blockquote, the verbatim guidance, absolute links.
- Robots headers: noindex on the account routes, none on public routes; the
  anonymous 302s for search and documents. While summaries are available, a
  summary answers 200 without a session, and `noindex` when its `index` is
  false.
- A browser sweep of the indexable pages without a cookie: one canonical, one
  `h1`, the title and description limits, no CSP violation. It starts its own
  Nitro against a fixture API that answers status, taxonomy and
  `taxonomy/{code}` with success or 503; the shared e2e stub keeps
  answering 503.
