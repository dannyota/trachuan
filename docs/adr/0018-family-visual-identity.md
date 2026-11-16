# ADR 0018: Family visual identity and voice

Status: accepted 2026-11-16.

## Context

The owner wants every product he makes to read as one family, in look and in
wording. aboutme.vn, his resume product, chose its identity on 2026-11-15: a
product made by a person, warm and colourful; white ground with a tinted ink;
one flat brand fill; large rounded panels and pill buttons; one chrome typeface;
real objects instead of decoration; a first-person voice that ends with the
maker's own introduction and picture; no gradients, no motion on load; contrast
computed by script and pinned by a token test (aboutme ADR 0020, "The 2026-11-15
identity").

Tra Chuẩn looked like an official gazette: ink on paper, a red seal, stamps for
validity, grey neutral surfaces, small radii, an impersonal register. It is a
dense legal reading product: people and agents quote what it shows, so evidence
must stay exact and neutral.

The owner decided on 2026-11-16 that Tra Chuẩn keeps navy `#1E3A8A` as its brand
colour and the seal red `#B42318` for the seal and for danger, that wording is
as much part of the family as colour, and that the home page ends with his
introduction and picture, linking to aboutme.vn/danny.

## Decision

**Concept.** A legal library made by a person: the desk is friendly, the paper
is exact. The chrome (header, home, help, sources, sign-in, account, footer)
wears the family. The paper has two layers and neither speaks: the words of the
law (document titles, provision labels and headings, verbatim text) are quoted
exactly in a serif; the evidence the product states about them (validity stamps,
status basis, warnings, citations, relation labels, counts) is neutral and exact
in the chrome face.

**Taken from the family.**

- White ground; an ink tinted to the product's own hue; one flat brand fill used
  for at most one panel per page, primary actions and active states.
- Plus Jakarta Sans as the one chrome typeface; heavy, tight headings on home
  and page heroes.
- Large rounded panels on home and page heroes; pill buttons and chips
  everywhere.
- Real objects instead of decoration: the stored sample result and its real
  citation, never an illustration or an invented number.
- Colour changes in 150 ms and dialogs open and close; nothing animates on load;
  reduced motion turns it all off; no gradients, glows or glass.
- The voice: the product and the maker speak as "mình" to "bạn", in short
  sentences, stating only what is true today.
- The maker panel at the end of home.
- Contrast computed by script for every pair in both themes, pinned by a token
  test.

**Trachuan's own.**

- Navy is the brand fill; the ink, neutrals and tints derive from navy's hue.
  There is no sun tint, because yellow already marks matched words, and no
  second dark panel.
- The seal logo keeps its red; on navy the whole logo is white.
- Validity stamps keep their meaning, terms, colours, shape and uppercase. They
  are Tra Chuẩn's one mark with one meaning: the only small-radius rectangle and
  the only uppercase on a page.
- Noto Serif for the words of the law; Roboto Mono for document numbers.
- Dense screens (results, reader, tables) keep 16 px cards and today's text
  sizes.
- The voice steps back per content field: anything a person or an agent might
  quote as evidence, or act on for security, is neutral and exact.

**Rollout.** Home first, then the rest of the chrome within one release, then
the dense screens; each stage is one pull request with its token test and fit
test.

The rules live in [web visual system](../design/web-visual.md) and
[web voice](../design/web-voice.md).

## Consequences

- `apps/web/app/assets/tokens.css` changes ink, neutrals, surfaces, radii and
  adds the brand hover, hero, footer and ring tokens; the static error-page
  stylesheet follows them.
- Plus Jakarta Sans ships self-hosted with its OFL licence, so the CSP keeps
  `font-src 'self'`. Be Vietnam Pro stays as a fallback until screenshots of 12
  to 14 px Vietnamese on the dense screens are accepted, then it is removed.
- Input and button borders reach 3:1, which today's `#C6CBD2` does not.
- Interface copy is rewritten to the voice rules. Claims about questions, status
  and source links become exact.
- Font, home, sample, static-page and reader tests change with the stage that
  changes their behaviour.
- The owner's picture and name appear on trachuan.vn; the files are copied from
  aboutme and served from the same origin.
- The navy panel and the stamps are quieter than aboutme's pink, indigo and
  yellow. That gap is accepted: Tra Chuẩn's energy comes from type scale, the
  real result hanging over the navy panel, and the stamps, the only saturated
  marks besides the seal.

## Alternatives rejected

- **aboutme's pink-red as the brand fill.** The owner kept navy; a pink fill
  also sits beside the seal red and danger at a low contrast.
- **aboutme's indigo ink and neutrals copied.** The neutral stamp border would
  drop to 2.99:1 on the indigo dark card; navy-derived values keep every stamp
  pair passing unchanged.
- **Grey ink and neutrals kept.** They do not tie the ink to the product's hue,
  which is the family's pattern; the navy-tinted ink keeps the same separation
  from navy as the grey (1.76:1 against 1.73:1).
- **A sun tint.** Against the matched-word highlight it is 1.02:1.
- **Be Vietnam Pro kept for chrome.** A second sans adds weight without a role
  and hides the family's strongest signal.
- **Large radii on dense screens.** They waste corners in lists; 16 px still
  reads as the family next to pills.
- **A tilted sample card.** A legal citation stays straight.
- **First-person voice everywhere.** Results, the reader, stamps, warnings,
  citations, consent and security text are quoted or acted on and stay neutral.

## History

- 2026-11-16: accepted. The owner asked that his products share one energy and
  one wording, kept navy as Tra Chuẩn's brand colour with the seal and danger
  red unchanged, and approved the maker panel with his picture, name and link to
  aboutme.vn/danny. Reviewed by gpt-6.1-sol in two rounds.
