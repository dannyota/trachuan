# Web visual system

Decision: [ADR 0018](../adr/0018-family-visual-identity.md). Voice:
[web voice](web-voice.md). Routes and pages: [web](web.md).

Tra Chuẩn is a legal library made by a person: the desk is friendly, the paper
is exact. The chrome (header, home, help, sources, sign-in, account, footer)
uses the family look below. The words of the law (document titles, provision
labels and headings, verbatim text) are set in Noto Serif. The evidence the
product states (stamps, status basis, warnings, citations, relation labels,
counts) is set in the chrome face. Both are neutral (see
[web voice](web-voice.md)).

## Colour tokens

Tokens live in `apps/web/app/assets/tokens.css` as `--color-<name>`. Dark values
follow the device (`prefers-color-scheme`); there is no toggle.

| Token                                              | Light                                                                              | Dark                                                                               | Role                                                     |
| -------------------------------------------------- | ---------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------- | -------------------------------------------------------- |
| `page`                                             | `#FFFFFF`                                                                          | `#0C1222`                                                                          | page ground                                              |
| `card`                                             | `#FFFFFF`                                                                          | `#161E33`                                                                          | cards                                                    |
| `mist`                                             | `#F1F4FA`                                                                          | `#131A2E`                                                                          | quiet panels, bands, notes, the reader target, row hover |
| `sky`                                              | `#E3EAF8`                                                                          | `#1B2A52`                                                                          | the maker panel; never under a stamp                     |
| `ink`                                              | `#0F1524`                                                                          | `#EEF1F8`                                                                          | text, outline pills                                      |
| `muted`                                            | `#4D556A`                                                                          | `#A3ABC2`                                                                          | leads, descriptions, notes                               |
| `line`                                             | `#E1E5EE`                                                                          | `#283048`                                                                          | decorative borders and rules                             |
| `control`                                          | `#6F7A9B`                                                                          | `#6F7DA3`                                                                          | input, checkbox and secondary button borders             |
| `primary-fill`                                     | `#1E3A8A`                                                                          | `#3451B2`                                                                          | the brand panel, primary pills, active states            |
| `primary-hover`                                    | `#172F70`                                                                          | `#2C469C`                                                                          | hover on primary fills                                   |
| `primary`                                          | `#1E3A8A`                                                                          | `#9DB4F5`                                                                          | navy words, links and thin strokes on a ground           |
| `on-primary`                                       | `#FFFFFF`                                                                          | `#FFFFFF`                                                                          | text and the logo on `primary-fill`                      |
| `primary-hint`                                     | `#D3DCF6`                                                                          | `#D3DCF6`                                                                          | secondary hints on `primary-fill`                        |
| `hero-field`, `hero-field-ink`, `hero-placeholder` | `#FFFFFF`, `#0F1524`, `#4D556A`                                                    | `#EEF1F8`, `#0F1524`, `#4D556A`                                                    | the query field on `primary-fill`                        |
| `hero-pill`, `hero-pill-ink`                       | `#FFFFFF`, `#1E3A8A`                                                               | same                                                                               | actions on `primary-fill`                                |
| `footer`, `footer-ink`, `footer-muted`             | `#0F1524`, `#FFFFFF`, `#C3CADB`                                                    | `#131A2E`, `#EEF1F8`, `#A3ABC2`                                                    | the footer band                                          |
| `ring`                                             | `#0F1524`                                                                          | `#9DB4F5`                                                                          | focus ring                                               |
| `ring-inverse`                                     | `#FFFFFF`                                                                          | `#FFFFFF`                                                                          | focus ring on `primary-fill`                             |
| `footer-ring`                                      | `#FFFFFF`                                                                          | `#9DB4F5`                                                                          | focus ring on the footer band                            |
| `seal`, `seal-footer`                              | `#B42318`, `#E5604F`                                                               | `#E5604F`, `#E5604F`                                                               | the logo seal; on the footer band                        |
| `danger`, `danger-bg`, `danger-line`               | `#B42318`, `#FEF3F2`, `#F3C7C1`                                                    | `#E5604F`, `#2A1412`, `#5A2620`                                                    | destructive actions and errors                           |
| `highlight`, `highlight-ink`                       | `#FFF1A8`, `#0F1524`                                                               | `#4A3F12`, `#F5ECC0`                                                               | matched words in result text                             |
| `note-warn`                                        | `#FDF6E9`                                                                          | `#2B2110`                                                                          | warning notices                                          |
| stamp inks and fills                               | `#0B6E63`/`#E3F4F0`, `#8A4B08`/`#FDF1DC`, `#1D4ED8`/`#E4ECFE`, `#4A5160`/`#ECEEF1` | `#5ED0BD`/`#0F2C28`, `#F0B565`/`#33230B`, `#9DB8FF`/`#16224A`, `#B3B9C4`/`#23272E` | in force, partly in force, not yet in force, expired     |
| `stamp-neutral-border`                             | `#6B7280`                                                                          | `#6B7280`                                                                          | expired, suspended and unknown stamp borders             |

`theme-color` is `page`. The fixed 404 and unavailable pages use a static
stylesheet whose values equal these tokens in both themes.

## Colour rules

- `primary-fill` fills at most one panel per page, primary pills and active
  states. A navy pill never sits on the navy panel; there it is `hero-pill`.
- Text links are always underlined (1 px, offset 3 px; 2 px, offset 5 px on
  home), because `ink` and `primary` are only 1.76:1 apart in light and 1.81:1
  in dark. A navy action is a filled pill, never navy text standing alone.
- Inside the navy panel the query field has no border: its `hero-field` fill is
  the boundary. Suggestions are outline pills in `on-primary` with a 2 px
  border; the checkbox border and mark are `on-primary`; focus is a 3 px
  `ring-inverse` ring with a 3 px offset.
- The seal never sits on navy. On the navy panel the whole logo is `on-primary`.
  On the footer band the wordmark is `footer-ink` and the seal `seal-footer`. On
  `page` and `mist` the seal is `seal`.
- A stamp never sits on `sky`. Stamps keep their meaning, terms, colours,
  uppercase and 6 px radius; nothing else uses that shape or uppercase.
- There are no gradients, glows or glass.

## Stamps and logo

Validity stamps differ by text, border and fill, not colour alone: in force,
partly in force, not yet in force, expired (struck through) and unknown (dashed
border); suspended uses the expired palette without the strike. The logo and
icons are the seal with "TC" (a solid red circle with "T" at 16 px), generated
with the Open Graph image by `apps/web/scripts/make-icons.mjs`; the wordmark
takes the text colour of its ground.

## Type

| Role                                       | Face                                     | Size and weight                                                                                           |
| ------------------------------------------ | ---------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| Chrome, headings, stamps, warnings, labels | Plus Jakarta Sans                        | body 500 16/1.6; descriptions 15/1.5; notes 14/1.5; labels 600; buttons 700                               |
| Home and page hero h1                      | Plus Jakarta Sans 800, tracking -0.025em | 40/1.18, 48 from 48rem, 60/1.15 from 80rem                                                                |
| Section h2                                 | Plus Jakarta Sans 800, tracking -0.02em  | 32/1.25, 40/1.18 from 64rem                                                                               |
| Headings on dense screens                  | Plus Jakarta Sans 700                    | today's sizes                                                                                             |
| Words of the law                           | Noto Serif                               | reader text 17/1.8; result text 15.5/1.75; reader title 34 (24 under 768 px); provision headings as today |
| Document numbers                           | Roboto Mono 500, ligatures off           | as today                                                                                                  |

Fonts are self-hosted woff2 with `font-display: swap`; the CSP keeps
`font-src 'self'`. Plus Jakarta Sans ships with its OFL licence and covers
U+1EA0 to U+1EF9, Đ đ Ơ ơ Ư ư Ă ă, U+20AB and the Vietnamese combining marks.
Its stack is `'Plus Jakarta Sans', sans-serif` on every page, the static error
pages and the OAuth consent page; there is no second sans. Only the Plus Jakarta
Sans face goes above 700; Noto Serif and Roboto Mono declare 400 to 700.

## Home

Home is, from the top: the white header; the navy panel holding the headline,
the lead, the query field, the suggestion pills and the sign-in hint; the stored
sample result hanging over the panel's bottom edge, straight, with its real
stamp and citation pill and no other decoration; a `mist` section on how a
result reads, with the facts as hairline rows; a white section with the live
coverage counts and their release; the connect section; the `sky` maker panel;
the footer band. Actions come in pairs, a filled pill then an outline pill ("Tra
cứu", "Xem cách kết nối").

## Shape and space

| Element                                     | Radius                           |
| ------------------------------------------- | -------------------------------- |
| Home and page hero panels                   | 28, 36 from 48rem, 44 from 64rem |
| Cards on home, connect, account and sign-in | 22                               |
| Result cards and reader rail cards          | 16                               |
| Dialogs                                     | 14                               |
| Inputs                                      | 10                               |
| Buttons and chips, relation chips included  | pill                             |
| Stamps                                      | 6                                |

Spacing follows an 8 px module. Buttons are at least 44 px tall, 52 px on home.
The stored sample result on home hangs over the bottom edge of the navy panel,
straight, with the shadow
`0 36px 56px -28px rgba(0,0,0,.5), 0 2px 6px rgba(0,0,0,.16)`; no other element
has a shadow. The reader's text has no container.

## Result cards

A search result card has no footer band. Its head carries the stamp with its
as-of line, the document number, the type and issuer, the actions ("Đọc trong
văn bản", "Sao chép trích dẫn", the source link) and the rank. From 1200 px the
head is one line and the type and issuer are cut with an ellipsis; below 1200 px
it is two lines, the stamp and number and then the actions and rank, and the
type and issuer move in front of the document title. The document number is
never cut: when it does not fit beside the stamp it takes its own line, and it
wraps only when it is longer than the card. No text in the card is smaller than
12 px, and the actions are 13 px. The document title follows on one line with an
ellipsis, then the provision heading in Noto Serif and the text. In the
collapsed card, a first text line equal to the shown heading after white-space
normalization alone is not shown; the snippet itself is never changed, and the
expanded text and every copy action keep the full text. Topic chips close the
card on one row. At 1366 px a card with three text lines is at most 220 px tall.

## Motion

Home changes colour in 150 ms and nothing else moves. Other pages also open and
close dialogs. Nothing animates on load; stamps never animate; reduced motion
turns all of it off.

## Contrast

WCAG 2 contrast for every pair in use, in both themes: text at least 4.5:1;
marks, borders and focus rings at least 3:1. The token test in `apps/web` reads
`tokens.css` and asserts these values and minimums; this table is its source.

| Minimum | Foreground             | Ground              | Light | Dark  |
| ------- | ---------------------- | ------------------- | ----- | ----- |
| 4.5     | `ink`                  | `page`              | 18.21 | 16.51 |
| 4.5     | `ink`                  | `card`              | 18.21 | 14.65 |
| 4.5     | `ink`                  | `mist`              | 16.53 | 15.29 |
| 4.5     | `ink`                  | `sky`               | 15.08 | 12.39 |
| 4.5     | `ink`                  | `note-warn`         | 16.94 | 13.99 |
| 4.5     | `muted`                | `page`              | 7.44  | 8.14  |
| 4.5     | `muted`                | `card`              | 7.44  | 7.23  |
| 4.5     | `muted`                | `mist`              | 6.75  | 7.54  |
| 4.5     | `muted`                | `sky`               | 6.16  | 6.11  |
| 4.5     | `muted`                | `note-warn`         | 6.92  | 6.90  |
| 4.5     | `highlight-ink`        | `highlight`         | 15.96 | 8.77  |
| 4.5     | `on-primary`           | `primary-fill`      | 10.36 | 7.08  |
| 4.5     | `on-primary`           | `primary-hover`     | 12.52 | 8.53  |
| 4.5     | `primary-hint`         | `primary-fill`      | 7.57  | 5.17  |
| 4.5     | `primary`              | `page`              | 10.36 | 9.14  |
| 4.5     | `primary`              | `card`              | 10.36 | 8.11  |
| 4.5     | `primary`              | `mist`              | 9.40  | 8.46  |
| 4.5     | `primary`              | `sky`               | 8.58  | 6.86  |
| 4.5     | `footer-ink`           | `footer`            | 18.21 | 15.29 |
| 4.5     | `footer-muted`         | `footer`            | 11.09 | 7.54  |
| 4.5     | `danger`               | `page`              | 6.57  | 5.44  |
| 4.5     | `danger`               | `card`              | 6.57  | 4.83  |
| 4.5     | `danger`               | `danger-bg`         | 6.05  | 5.07  |
| 4.5     | `ink-in-force`         | `stamp-in-force-bg` | 5.39  | 7.96  |
| 4.5     | `ink-partial`          | `stamp-partial-bg`  | 6.08  | 8.30  |
| 4.5     | `ink-not-yet`          | `stamp-not-yet-bg`  | 5.66  | 7.88  |
| 4.5     | `ink-expired`          | `stamp-expired-bg`  | 6.85  | 7.60  |
| 4.5     | `ink-expired`          | `page`              | 7.96  | 9.47  |
| 3       | `control`              | `page`              | 4.26  | 4.57  |
| 3       | `control`              | `card`              | 4.26  | 4.05  |
| 3       | `control`              | `mist`              | 3.86  | 4.23  |
| 3       | `control`              | `sky`               | 3.53  | 3.43  |
| 3       | `ring`                 | `page`              | 18.21 | 9.14  |
| 3       | `ring`                 | `card`              | 18.21 | 8.11  |
| 3       | `ring`                 | `mist`              | 16.53 | 8.46  |
| 3       | `ring`                 | `sky`               | 15.08 | 6.86  |
| 3       | `ring-inverse`         | `primary-fill`      | 10.36 | 7.08  |
| 3       | `footer-ring`          | `footer`            | 18.21 | 8.46  |
| 3       | `on-primary`           | `primary-fill`      | 10.36 | 7.08  |
| 3       | `hero-field`           | `primary-fill`      | 10.36 | 6.26  |
| 4.5     | `hero-field-ink`       | `hero-field`        | 18.21 | 16.11 |
| 4.5     | `hero-placeholder`     | `hero-field`        | 7.44  | 6.58  |
| 3       | `hero-pill`            | `primary-fill`      | 10.36 | 7.08  |
| 4.5     | `hero-pill-ink`        | `hero-pill`         | 10.36 | 10.36 |
| 3       | `seal`                 | `page`              | 6.57  | 5.44  |
| 3       | `seal`                 | `mist`              | 5.97  | 5.04  |
| 3       | `seal-footer`          | `footer`            | 5.31  | 5.04  |
| 3       | `primary`              | `mist`              | 9.40  | 8.46  |
| 3       | `ink-in-force`         | `page`              | 6.13  | 9.98  |
| 3       | `ink-in-force`         | `card`              | 6.13  | 8.86  |
| 3       | `ink-in-force`         | `mist`              | 5.57  | 9.25  |
| 3       | `ink-partial`          | `page`              | 6.79  | 10.22 |
| 3       | `ink-partial`          | `card`              | 6.79  | 9.07  |
| 3       | `ink-partial`          | `mist`              | 6.17  | 9.47  |
| 3       | `ink-not-yet`          | `page`              | 6.70  | 9.54  |
| 3       | `ink-not-yet`          | `card`              | 6.70  | 8.47  |
| 3       | `ink-not-yet`          | `mist`              | 6.08  | 8.84  |
| 3       | `ink-expired`          | `page`              | 7.96  | 9.47  |
| 3       | `ink-expired`          | `card`              | 7.96  | 8.40  |
| 3       | `ink-expired`          | `mist`              | 7.23  | 8.77  |
| 3       | `stamp-neutral-border` | `page`              | 4.83  | 3.86  |
| 3       | `stamp-neutral-border` | `card`              | 4.83  | 3.43  |
| 3       | `stamp-neutral-border` | `mist`              | 4.39  | 3.58  |

Pairs below are not used where the failing theme is marked. Each row says what
is used instead.

| Pair                            | Light | Dark | Rule                                                                                             |
| ------------------------------- | ----- | ---- | ------------------------------------------------------------------------------------------------ |
| `control` on `primary-fill`     | 2.43  | 1.73 | both themes: no control border on navy; the hero field is bounded by its `hero-field` fill       |
| `card` on `primary-fill`        | 10.36 | 2.34 | dark: never the only boundary of a control; the sample card may overlap the panel in both themes |
| `seal` on `primary-fill`        | 1.58  | 2.06 | both themes: on navy the whole logo is `on-primary`                                              |
| `ring` on `footer`              | 1.00  | 8.46 | light: the footer uses `footer-ring`                                                             |
| `stamp-neutral-border` on `sky` | 4.00  | 2.90 | both themes by rule: a stamp never sits on `sky`, where the not-yet fill edge is 1.02:1 in light |

The dark `primary-fill` against the dark `page` is 2.64:1. The panel edge
carries no information and buttons are named by their labels, so this meets WCAG
1.4.11; the exception never covers a state indicator or an input boundary.
