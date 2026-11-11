# ADR 0014: OCR on the write path

Status: accepted (owner decision 2026-11-11).

## Context

299 of the 562 documents without a source tree exist only as scanned PDFs or
PDFs with an unusable text layer, about 10,500 pages; 263 are consolidated
texts, the current reading of amended State Bank rules
([inventory](../research/vn-text-inventory-2026-11.md)). Without OCR they cannot
be found. banhmi, the predecessor, read the same files with Google Cloud Vision
and kept the output; its cache holds OCR text keyed by the PDF's SHA-256 for 266
of those documents. [ADR 0001](0001-reuse-banhmi-data-only.md) lets trachuan
reuse banhmi's data, not its code.
[ADR 0002](0002-evidence-only-vietnamese-queries.md) forbids models on the
serving path only.

## Decision

1. OCR runs on the write path only (the corpus pipeline on the MacBook), never
   on the host and never at query time.
2. The engine is Google Cloud Vision document text detection. Output is stored
   in an OCR store keyed by the PDF's SHA-256 with engine, version and request
   time. banhmi's stored Google Vision output is imported into the store as data
   when the file hash matches (engine `vision-ocr/builtin-latest`, origin
   `banhmi`); only PDFs absent from the store are sent to the service.
3. Every OCR text gets a cross-check before it is used: a hosted language model
   (Claude or an OpenAI model, chosen by the owner) reads the page images with
   the OCR text and reports disagreements in document numbers, dates, article
   labels and amounts. Disagreements are stored with the text, set the gap
   `ocr_flagged` on the document and enter a review file; the model never
   rewrites the stored text.
4. OCR text ships as citable evidence, marked `text_source = ocr` on every
   result, under the fail-closed parsing of
   [text extraction](../design/text-extraction.md).
5. Legal text of public documents is the only content sent to either service. No
   user question or personal data is sent.

## Consequences

- Cost. Vision lists the first 1,000 units a month free and 1.50 USD per 1,000
  pages after that ([pricing](https://cloud.google.com/vision/pricing), read
  2026-11-11). Fresh OCR after reuse was measured at 48 documents and 1,342
  pages, about 0.51 USD. The cross-check reads every OCR page, including the
  reused ones (about 10,500 pages); its cost depends on the model the owner
  picks and is measured on a 100-page sample before the full pass.
- Credentials for both services live on the MacBook outside the repository and
  are named by environment variables in the runbook; the pipeline fails closed
  when they are absent, and reused OCR needs none.
- A new engine or model version is a new store entry, never an overwrite, so a
  release can always name the OCR reading it was built from.
- Risks kept visible: OCR misreads of numbers and dates; the cross-check and the
  planned user report feature are the two controls, and manual corrections the
  repair.

## Alternatives

- Local engines (Apple Vision, Tesseract `vie`): no cost and no data leaves the
  machine, but unmeasured accuracy on Vietnamese legal scans; the owner chose
  the engine banhmi already proved on these files.
- Reading PDF text layers: 203 measured layers are garbage and only 24 are good;
  not worth a second parser.

## History

- 2026-11-11: accepted. The owner trusts Google OCR checked by a second model
  and allows OCR text as citable evidence.
