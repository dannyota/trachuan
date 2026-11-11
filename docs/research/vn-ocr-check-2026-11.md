# OCR cross-check pilot, November 2026

Evidence for [OCR cross-check](../design/ocr-check.md) and
[ADR 0014](../adr/0014-ocr-on-the-write-path.md): what a hosted model finds when
it compares stored Google Vision text with the PDF, what it costs, and which
model to use. Measured on 2026-11-11 against run `vn/20261110_6`, 282 imported
OCR texts, the first 10 documents the command selects (16 to 97 pages each),
direct mode, medium effort, a spend limit of 5 USD per checker.

## Result

Use `claude-opus-5-5/medium/ocr-check-2`. On the same documents Opus reported 23
differences, all confirmed against the page; Sonnet reported 12, of which 2 were
wrong and 5 trivial, and 3 of its 10 answers failed validation. The full pass
over about 10,500 pages costs about 170 USD in direct mode at the measured
0.0165 USD a page, half of that in batch mode.

## Version 1: the PDF as a document

Version 1 sent the PDF as a document block. The API gives the model each page's
embedded text layer beside the page image, and for most of these scans that
layer is the output of an earlier, non-Vietnamese OCR pass.

| Checker                                | Attempts | Complete | Failed       | Input tokens | Cost (USD) |
| -------------------------------------- | -------- | -------- | ------------ | ------------ | ---------- |
| `claude-sonnet-5-5/medium/ocr-check-1` | 10       | 8        | 2 (`schema`) | 1,128,763    | 2.29       |

All 8 complete answers were `flagged`, 19 items. In two documents the quoted
"OCR" readings were garbled strings (`Th6ng tt s6 o3/2016`, `Phu lpc lB`) that
do not occur in the stored OCR text: the model had read the embedded layer as
the text under check. Other items reported identical readings as differences or
concerned stamps and handwriting. The real findings were three form codes in
decree 142/2026/NĐ-CP. The two failures recorded no cause. Version 1 is not
usable.

## Version 2: page images and verifiable quotes

Version 2 renders pages itself (150 dpi, grayscale), sends images only, requires
every quoted OCR reading to occur in the stored text, drops equal readings and
records which rule a failed answer broke.

| Checker                                | Attempts | Complete | Failed | Flagged | Clean | Input tokens | Output tokens | Cost (USD) |
| -------------------------------------- | -------- | -------- | ------ | ------- | ----- | ------------ | ------------- | ---------- |
| `claude-sonnet-5-5/medium/ocr-check-2` | 10       | 7        | 3      | 4       | 3     | 1,267,842    | 2,546         | 2.56       |
| `claude-opus-5-5/medium/ocr-check-2`   | 7        | 7        | 0      | 6       | 1     | 1,022,421    | 7,128         | 4.23       |

Opus stopped after 7 documents at the spend limit. Input is the same for both
models: about 4,000 tokens a page (97 pages, 384,667 tokens), against about
2,700 a page for version 1. Sonnet's failures were two answers with an invalid
page number (`schema:items[n].page`) and one that broke the reading rules
(`item_readings`); failed attempts are billed (0.45 USD of the 2.56).

Cost per page from the complete Opus attempts: 4.23 USD for 257 pages, 0.0165
USD. Sonnet: 2.56 USD for the 264 pages it completed, failures included, 0.0097
USD.

## Every item read against its page

All 35 items of version 2 were checked by rendering the cited page and reading
it beside the OCR text.

| Model  | Items | True | True, trivial | False |
| ------ | ----- | ---- | ------------- | ----- |
| Opus   | 23    | 22   | 1             | 0     |
| Sonnet | 12    | 5    | 5             | 2     |

What Opus found, by kind: 10 document or form numbers, 9 article or point
labels, 2 missing passages, 1 amount, 1 trivial extra text. The findings that
matter for citations:

- A sign: the page says column 7 takes `(+)` for an increase; the OCR text has
  `(-)` for both increase and decrease. Both models found it.
- Point labels: in one 24-page consolidated text the OCR reads point `l)` as
  `1)` or `I)` in six labels and cross-references ("Điểm 1 Khoản 1 Điều 13" for
  "Điểm l"), reads clause `1a.` as `la.`, and drops the label `b)` of a point.
  Only Opus found these.
- Form codes of decree 142/2026/NĐ-CP: `AI01b`, `AI02`, `AI06a`, `AI06b`,
  `AI07a`, `AI07b`, `AI08a`, `AI08b` read as `AJ016`, `AJ02`, `A06a`, `A106b`,
  `A107a`, `A107b`, `AK08a`, `A108b`. Opus found eight, Sonnet three.
- `(ii)` for `(iii)` in a list, and the slash of `24/VBHN-NHNN` read as `N` in
  two authentication blocks.

Sonnet's two false items were both `missing_text` claims where the passage is in
the OCR text and only a footnote mark or a row of dots is absent. It also filed
one-letter spelling slips as `extra_text` with an empty page reading and named
one wrong page. Opus's page numbers and quotes were all exact.

Limits seen: a short quote such as `S` or `59` occurs many times in a text, so
the occurrence check proves existence, not position; three Opus items sit in
authentication blocks or a footnote and could be argued out of scope; 150 dpi
was enough to tell `l` from `1`, confirmed on larger crops.

## What the check does not do

It reports; it never rewrites. Every difference above stays in the stored OCR
text until a correction mechanism exists, so a flagged document is indexed with
its misread label and the gap `ocr_flagged`. The review file lists the items for
a person. Whether confirmed items should patch the text is an open design
question and belongs to the next slice.
