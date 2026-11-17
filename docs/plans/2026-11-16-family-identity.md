# Family Identity Plan (Plan 17)

> **For agentic workers:** Routes (`instructions/workflow.md`): each stage is routine implementation by Codex `implementer` (`gpt-6.1-terra` high) in `apps/web/` only, reviewed by `reviewer` (`gpt-6.1-sol` high). The primary runs `make pre-push`, `make web-test` and `make e2e` and takes the screenshots named below.

**Goal:** apps/web wears the family identity of [ADR 0018](../adr/0018-family-visual-identity.md) and speaks in its voice, in three stages, each one pull request with its token test and fit test.

**Spec:** [web visual system](../design/web-visual.md), [web voice](../design/web-voice.md), [web](../design/web.md). The spec wins over this plan.

## Stage 0: cleanup in the first apps/web PR

- Remove the unused dependencies `class-variance-authority`, `clsx` and `tailwind-merge` from `apps/web/package.json` and the lockfile; no app code imports them.

## Defects fixed on the way

| Defect | Evidence | Stage |
|-|-|-|
| Home `sampleText` says "hết hiệu lực một phần", not a stamp term | `apps/web/i18n/home.ts:16`; stamp term `i18n/vi.ts:44` | 1 |
| "Câu hỏi không bao giờ được lưu." / "Questions are never stored." contradicts the disclosed memory caches | `i18n/pages.ts:16`, `:8`; privacy `i18n/legal.ts:11`; `docs/design/web.md:15` | 2 |
| Unused keys `loginGoogle` and `legalQuestions` | `i18n/pages.ts:8`, `:16`; `i18n/vi.ts:36`, `i18n/en.ts:39`; `/login` renders `authMessages().google` (`app/pages/login.vue:42`) | 2 |
| Input and secondary button borders `#C6CBD2` at 1.63:1 (1.70:1 dark) | `app/assets/tokens.css`, `base.css:12-14` | 2 |
| The account security card nests padding; its heading sits 20 px right of the others | `app/assets/areas/auth.css` `.account-security` inside `.account-card` | 2 |

## Stages

Each stage owns every copy change and every existing test whose behaviour it changes first. The copy test (no em dash, no arrow, no emoji) covers only the files a stage has rewritten and widens stage by stage; `i18n/reader.ts` keeps its arrows until stage 3.

| Stage | Changes | Existing tests it updates | New tests | Exit |
|-|-|-|-|-|
| 1 Home | Tokens and Plus Jakarta Sans scoped to the home route; the navy hero with its field, pills, suggestions, checkbox and focus ring; the home composition of `docs/design/web-visual.md` "Home" (sample result over the panel edge, facts as hairline rows on `mist`, coverage on white, connect, the `sky` maker panel with the two WebP files); copy in `i18n/home.ts` | `e2e/home.spec.ts:31` and `tests/home.test.ts:31`, `:38` (sign-in hint); `e2e/home.spec.ts:49-53` (the hero stops being a full-width band); `tests/home-sample.test.ts:38` (the link line no longer contains "vbpl.vn"; the conditional-link test stays, keyed to the new line); `tests/fonts.test.ts:15`, `:24`, `:39` become family-specific: the Plus Jakarta Sans face declares and may use 400 to 800, Be Vietnam Pro, Noto Serif and Roboto Mono keep `400 700` (`app/assets/fonts.css:1-3`), and no rule outside the Plus Jakarta face goes above 700 | Token test: reads the tokens file and asserts every value in both themes and every pair of the contrast table in `docs/design/web-visual.md` at its minimum. Copy test on `i18n/home.ts`. Playwright on home at 360, 390, 768, 1024, 1366, light and dark: no horizontal scroll, panel radius per breakpoint, pills at least 44 px, the sample card fully visible and over the panel edge, `hero-field` fill and the white focus ring, no transitions under reduced motion, axe clean | all checks green |
| 2 Chrome | Global tokens (ink, neutrals, `mist` and `sky` replacing `surface` and `note`, `control`, rings, radii, pills); `theme-color`; header (navy sign-in pill, active state); the footer band and its focus ring; sign-in pages, account, connect, sources, topics and legal pages; `public/static-page.css`; copy in `i18n/shell.ts`, `pages.ts`, `auth.ts`, `account.ts`, `vi.ts` and `en.ts` (unavailable), `server/utils/not-found.ts`; delete `loginGoogle` and `legalQuestions`; stack `'Plus Jakarta Sans', BeVietnam, sans-serif`; the re-sign-in notice and resume of `docs/design/web.md` account security, with its copy in `i18n/auth.ts` and `/login`; ships within one release of stage 1 | `tests/display-name.test.ts:30` (login h1); `tests/static-page.test.ts:6-7`, `tests/response-security.test.ts:135`, `:240`, `tests/topic-render.test.ts:52` (404 text); `tests/unavailable-page.test.ts:24`, `tests/topic-render.test.ts:55`, `:89`, `:124`, `e2e/topic-navigation.spec.ts:43` (unavailable text); `e2e/search-states.spec.ts:58` (unavailable title); `e2e/reader-design.spec.ts:14` dark value (the dark target surface is `var(--color-surface)`); `e2e/responsive-sweep.spec.ts`, `layout.spec.ts`, `pages-layout.spec.ts`, `auth-pages.spec.ts`, account specs; `e2e-stack/accessibility.spec.ts` in both themes | Token test widened to global tokens; a parity test of `static-page.css` against the tokens in both themes; copy test widened to the files above; Playwright for the re-sign-in notice (no navigation before the button, the `/login` reason line, resume of each action, an unknown reason and an action older than 10 minutes ignored); a sweep of the restyled pages at 360, 390, 768 and 1366 in both themes | Screenshots of Vietnamese at 12 to 14 px on the search filters, result card head and reader rail, light and dark, accepted by the leader; then a follow-up PR removes the Be Vietnam Pro file and fallback and updates `tests/fonts.test.ts:5-10`, `:15`, `:24` and `public/static-page.css` |
| 3 Dense screens | Search and reader: serif provision headings, pill controls and relation chips, 16 px cards, the target on `mist` with the navy rule; the filter load error's "Thử lại" as an underlined text button; copy in `i18n/search.ts`; arrows in `i18n/reader.ts:4-5`, `:10-11` and `app/components/ResultCard.vue:96` replaced by icons | `e2e/search-states.spec.ts:21`, `:25`; `tests/i18n.test.ts:26`; `e2e/reader-design.spec.ts:14` light value; `e2e/search-layout.spec.ts` | Stamp and highlight pairs in the token test; copy test widened to `search.ts` and `reader.ts`; sweep 360 to 1366 in both themes | all checks green |

The home `og:description` and per-page head tags belong to the discovery work, not to this plan.

## Copy

Stage 1, `i18n/home.ts`:

| Key | vi | en |
|-|-|-|
| eyebrow | removed | removed |
| title | Hỏi bằng tiếng Việt, đọc nguyên văn từ văn bản gốc. | Ask in Vietnamese. Read the law in its own words. |
| intro | Chào bạn, mình là Tra Chuẩn. Mình không viết câu trả lời. Mình tìm các đoạn trong văn bản gốc và đưa nguyên văn, kèm số hiệu và trạng thái hiệu lực ghi trong dữ liệu. | Hi, I'm Tra Chuẩn. I don't write answers. I find passages in the source documents and show them word for word, with the document number and the status recorded in the dataset. |
| signIn | Bạn cần đăng nhập để tra cứu. Mình không ghi câu hỏi của bạn vào ổ đĩa hay nhật ký. | Sign in to search. I never write your questions to disk or logs. |
| sampleTitle | Một kết quả thật, từ bản phát hành ngày {date}. | A real result from the {date} release. |
| sampleText | Mỗi kết quả là một đoạn trích từ văn bản gốc, không phải cả văn bản. Con dấu là trạng thái hiệu lực ghi trong dữ liệu của Tra Chuẩn, cùng căn cứ của trạng thái đó; ở kết quả mẫu này, dòng bên cạnh con dấu ghi trạng thái tính đến ngày nào. Cảnh báo hiện ngay trên khoản khi một văn bản khác đã sửa đổi hoặc bãi bỏ nó. | Each result is a passage from the source document, not the whole document. The stamp is the document's status as recorded in Tra Chuẩn's dataset, with its stated basis; on this sample, the line beside the stamp gives the date the status applies to. A warning appears right on the clause when another document has amended or repealed it. |
| check1, check2, check3 | Nguyên văn, không diễn giải. / Sao chép trích dẫn bằng một lần bấm: {citation} / Liên kết về bản gốc, khi nguồn có công bố. (check3 only when the sample has an https source URL, as today) | The original text, never paraphrased. / Copy the citation in one click: {citation} / A link to the original, when the source publishes one. |
| scope, scopeLink | Mình tập trung vào ngân hàng, tài chính, công nghệ thông tin và dữ liệu cá nhân, nên chưa có mọi văn bản. / Xem những gì còn thiếu | I focus on banking, finance, IT and personal data, so not every document is here yet. / See what's missing |
| connectTitle, connectText, connectLink | Dùng Tra Chuẩn trong trợ lý AI của bạn. / Kết nối Claude, ChatGPT hoặc trợ lý khác hỗ trợ MCP. Trợ lý có thể tra cứu, đọc nguyên văn và trích kèm số hiệu văn bản. Bạn cấp quyền chỉ đọc và thu hồi lúc nào cũng được. / Xem cách kết nối | Use Tra Chuẩn from your AI assistant. / Connect Claude, ChatGPT or another MCP-capable assistant. It can search, read the original text and quote it with the document number. You grant read-only access and can revoke it any time. / See how to connect |
| maker (new) | Chào bạn, mình là Danny. / Mình từng hỏi trợ lý AI về luật và nhận được câu trả lời rất tự tin, nhưng không chỉ ra được điều khoản nào. Nên mình làm một chỗ chỉ đưa nguyên văn, để bạn tự kiểm tra được. Trang còn mới và chưa đủ mọi văn bản; những gì còn thiếu mình ghi ở trang Nguồn dữ liệu. / Tra cứu, Xem CV của mình, Gửi góp ý | Hi, I'm Danny. / I once asked an AI assistant about the law. It answered with total confidence and couldn't point to a single clause. So I built a place that shows only the original text and lets you check it for yourself. The site is new and doesn't hold every document yet; the sources page lists what's missing. / Search, See my resume, Send feedback |
| maker alt text (confirmed by the owner 2026-11-16) | Tranh vẽ chân dung Danny, bên cạnh là vợ và con anh trong khung hình trái tim | Drawn portrait of Danny, with his wife and child in a heart-shaped frame |

Maker picture: copy aboutme `apps/web/public/landing/maker-avatar-320.webp` (320x320) and `maker-avatar-640.webp` (640x640) unchanged into `apps/web/public/`; `srcset` with `sizes` "(min-width: 64rem) 232px, (min-width: 48rem) 176px, 168px"; circle, `object-fit: cover`, 6 px white border.

Stage 2:

| Key | vi | en |
|-|-|-|
| `pages.ts` connectIntro | Tra Chuẩn là một máy chủ MCP. Trợ lý của bạn có thể tra cứu, đọc văn bản và trích nguyên văn kèm số hiệu và trạng thái hiệu lực. Lần đầu kết nối, bạn đăng nhập và cấp quyền chỉ đọc; bạn thu hồi lúc nào cũng được trong trang Tài khoản. | Tra Chuẩn is an MCP server. Your assistant can search, read the text and quote it with the document number and status. The first time, you sign in and grant read-only access; revoke it any time from your account. |
| `pages.ts` sourcesIntro | Mọi văn bản ở đây lấy từ nguồn công khai của Nhà nước. Dưới đây là các nguồn, bản phát hành hiện tại và những gì mình chưa có. | Every document here comes from a public government source. Below are the sources, the current release and what I don't have yet. |
| `pages.ts` missing | Những gì mình chưa có | What I don't have yet |
| `pages.ts` loginTitle, loginIntro | Đăng nhập để tra cứu / Bạn cần tài khoản để tra cứu và đọc văn bản. Mình không ghi câu hỏi của bạn vào ổ đĩa hay nhật ký. | Sign in to search / You need an account to search and read documents. I never write your questions to disk or logs. |
| `not-found.ts` | Không có trang này. / Đường dẫn có thể đã cũ hoặc gõ nhầm. Bạn thử tra cứu, hoặc về trang chủ. | This page doesn't exist. / The link may be old or mistyped. Try a search, or go home. |
| `shell.ts` unavailableTitle, `vi.ts`/`en.ts` unavailable | Tra Chuẩn đang gián đoạn. / Bạn thử lại sau. | Tra Chuẩn isn't available right now. / Please try again later. |
| `account.ts` reportsEmpty | unchanged until the owner confirms "mình đọc từng báo lỗi" | unchanged |
| `auth.ts` re-sign-in notice, token | Để tạo mã, bạn cần đăng nhập lại. Xong bạn sẽ quay lại đây. | To create a token, sign in again. You'll come back here. |
| `auth.ts` re-sign-in notice, password-link | Để thêm mật khẩu, bạn cần đăng nhập lại. Xong bạn sẽ quay lại đây. | To add a password, sign in again. You'll come back here. |
| `auth.ts` re-sign-in notice, authenticator | Để thay đổi xác minh hai bước, bạn cần đăng nhập lại. Xong bạn sẽ quay lại đây. | To change two-step verification, sign in again. You'll come back here. |
| `auth.ts` re-sign-in button | Đăng nhập lại | Sign in again |
| `/login` reason line (`reason=reauth`) | Đăng nhập lại để tiếp tục trong trang Tài khoản. | Sign in again to continue on your account page. |

Stage 3, `i18n/search.ts`:

| Key | vi | en |
|-|-|-|
| idle | Gõ câu hỏi bằng tiếng Việt, hoặc một số hiệu như 50/2024/TT-NHNN. | Type a question in Vietnamese, or a document number such as 50/2024/TT-NHNN. |
| noEvidence, noEvidenceDetail | Chưa thấy khoản nào khớp. / Thử từ ngữ khác, hoặc tìm theo số hiệu văn bản. | No matching clause yet. / Try other words, or search by document number. |
| unavailable | Tra cứu đang gián đoạn. Bạn thử lại sau. | Search isn't working right now. Please try again later. |
| weak, weakDetail | unchanged (evidence signal) | unchanged |

Unchanged everywhere: account headings, account deletion text, stamp terms, warning sentences, citations.
