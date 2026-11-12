# Fresh OCR requests

This note owns the client that asks Google Cloud Vision for the text of a PDF
that the [OCR store](ocr-store.md) does not hold yet.
[ADR 0014](../adr/0014-ocr-on-the-write-path.md) owns the decision and the
engine; [OCR cross-check](ocr-check.md) owns the page renderer and the check
that every stored text still needs before `extract` uses it.

## Measured

Run `vn/20261111_2`: 45 documents end in `needs_ocr` with one candidate PDF that
has no store entry (190 MB together, largest 24 MB, 11 over 7 MB). The earlier
count for this group was 48 documents and 1,342 pages.

Vision limits ([quotas and limits](https://docs.cloud.google.com/vision/quotas),
read 2026-11-12): a JSON request holds at most 10 MB and an image at most 20 MB;
`files:annotate` reads at most 5 pages of one file per request;
`images:annotate` takes at most 16 images; 1,800 requests a minute. A PDF sent
inline is base64 text, about 37% larger than the file, so the 11 larger files
cannot go inline, and the asynchronous file method needs a Cloud Storage bucket.
banhmi met the same limits and sent one page image per request; the stored texts
that the store imported were made that way.

## Request

The client sends page images, one page per request, for every PDF:

- The PDF is read once from the run's verified fetch bundle, and its SHA-256
  must equal the hash the bundle lists; the same bytes are then counted,
  rendered and named in the store entry. A mismatch fails the document before
  any request.
- Pages are rendered in the cross-check's [sandbox](ocr-check.md#limits) as
  JPEG: 200 dpi, grayscale, quality 90, one file per page in the scratch
  directory. PNG encoding was the cost of the first run
  ([measured](#rendering-cost)).
- Render arguments are computed per page from its size, as the cross-check does,
  so the raster cap holds for every page. Consecutive pages with identical
  arguments form a group, and each group is one `pdftoppm` process (`-f`, `-l`).
  A document whose pages share one size, the usual case, is one process; a
  document of mixed sizes is a few. Nothing is rendered with another page's
  arguments.
- A render is complete only when every process exited with status 0 and the
  scratch directory holds exactly one file for each page number 1 to N, no other
  page file, each a JPEG that decodes in full. Page numbers are parsed as
  numbers, so zero padding does not affect order. N is the page count read with
  pdfcpu. Anything else fails the document and nothing is stored.
- Bounds: the render bound is 30 seconds plus 2 seconds per page for the
  document. The scratch directory keeps the cross-check's 256 MB ceiling for
  everything in it (the input, page files, anything else), enforced by the same
  monitor while a render process runs: the process is stopped when the ceiling
  is passed, and the document fails with outcome `render_too_large`. At about
  0.5 MB a page that admits roughly 400 pages of an ordinary scan, below the
  500-page limit; a larger document fails closed. Page files are read and sent
  one at a time, so memory holds only the pages in flight.
- One `images:annotate` request per page: feature `DOCUMENT_TEXT_DETECTION`,
  model `builtin/latest`, language hint `vi`, the JPEG bytes inline. One page
  per request keeps a failed page from discarding its neighbours and matches how
  the imported texts were made.
- The text of a page is the response's `fullTextAnnotation.text`, empty when the
  response has none. A response that carries an error status fails the page.
  Transient failures (unavailable, deadline, resource exhausted, internal) are
  retried three times after 1, 4 and 16 seconds; anything else fails at once.
- A document is stored only when every page succeeded: the page texts joined
  with one blank line, in page order, as the imported texts are. A document
  whose joined text is empty fails with outcome `empty_text`; the store holds no
  empty entry. The entry records origin `vision`, the page count and the engine
  id `vision-ocr/builtin-latest/poppler-200-jpeg`. The 43 entries read from PNG
  pages on 2026-11-12 keep their id `vision-ocr/builtin-latest/poppler-200`; the
  image form is part of the reading, so it is part of the id. A failed document
  stores nothing and is tried again by the next invocation.
- Requests in flight and requests per minute are in [budgets](budgets.md).

The client posts JSON to `https://vision.googleapis.com/v1/images:annotate`
([request format](https://docs.cloud.google.com/vision/docs/request)) with a
token from Application Default Credentials (`golang.org/x/oauth2/google`, scope
`cloud-vision`, credentials file in `GOOGLE_APPLICATION_CREDENTIALS`) and the
project in `GOOGLE_CLOUD_PROJECT` as the `x-goog-user-project` header. Google's
Go client library was set aside: it adds about twenty modules and raises the
gRPC and network library versions of the whole module for one JSON call. Neither
value is logged or written to an artifact. Only PDFs listed in a fetch bundle of
the run are sent: public legal text.

## Rendering cost

Measured 2026-11-12. The first run read 40 documents, 967 pages, in 19.2
minutes: 1.19 seconds per page, with each document rendered in full before its
first request. One 24-page A4 scan at 200 dpi, on a loaded machine (the ratios
matter, not the absolute values):

| Renderer                                             | Seconds | Per page |
| ---------------------------------------------------- | ------- | -------- |
| Contained renderer, PNG, one process per page        | 23.9    | 0.99     |
| Plain `pdftoppm`, PNG, one process per page          | 20.6    | 0.86     |
| Plain `pdftoppm`, PNG, one process for the document  | 20.6    | 0.86     |
| Plain `pdftoppm`, JPEG, one process for the document | 1.62    | 0.07     |

Rasterizing costs about 0.07 seconds a page and PNG compression about 0.8; the
sandbox and process starts cost little. A JPEG page is about 0.5 MB against 1.9
MB. Five documents of the first run failed with `render_failed` under the old
2-minute bound while the machine was overloaded.

Pages are sent after the document's render ends, not while it runs: at 0.07
seconds a page a 300-page document renders in about 20 seconds, and watching the
scratch directory for finished files would add a failure mode for a few seconds.
Rendering several documents in parallel, and MuPDF as the renderer, are the next
levers if a 300-page document still takes more than about a minute.

## Command

`corpus ocr vision <cc> <rel> --max-pages N [--limit-docs N] [--pdf SHA256] [--dry-run]`

- Selection: the run's documents whose frozen selection in `run_ocr_selection`
  is the gap `needs_ocr` with one PDF, and whose PDF has no `ocr_text` row for
  any engine id the extract recipe accepts. `--pdf` names one PDF of the run
  instead, whatever the store holds for other engines; it exists to compare
  engines on files that already have imported text.
- `--max-pages` is required and counts requests, because each request is one
  page and one billed unit. A document is started only when its page count fits
  in what is left. Every request, a retry included, takes one unit from the
  limit before it is sent; when none is left the document fails and nothing is
  stored. The command therefore never sends more requests than the limit.
- `--dry-run` renders nothing and sends nothing: it lists the selected PDFs with
  their page counts and the total.
- Output: one line per document (PDF SHA-256, pages, outcome) and totals:
  stored, failed, skipped for the limit, pages sent. No text is printed.

Cost is 1.50 USD per 1,000 pages after the first 1,000 a month (ADR 0014), so
this group costs under 1 USD; the limit is a guard against a wrong selection,
not a budget.

## After a fresh text

The text has no check yet, so `extract` still reports `needs_ocr`. The
cross-check command selects entries as `extract` does: for each PDF, the row of
the first engine id in the recipe's accepted list that exists. A PDF with only a
fresh entry is therefore checked; a comparison entry written with `--pdf` beside
an imported one is selected by neither `extract` nor the check, and exists only
to be compared. The extract recipe's list of accepted engine ids is the imported
id, then `vision-ocr/builtin-latest/poppler-200`, then
`vision-ocr/builtin-latest/poppler-200-jpeg`; that changes the recipe, so it
ships together with the checker id change, as one recipe change and one rerun of
extract.

## Checks

The primary runs these; they are requirements, not results.

- Rendering: a real 12-page PDF yields files for pages 1 to 12 from one process,
  in numeric order, each decodable and grayscale; a PDF whose pages have
  different MediaBox and CropBox sizes is rendered in groups and every page
  stays within the raster cap; a missing file, an extra file, a truncated JPEG
  and a non-zero exit each fail the document; a render that passes the scratch
  ceiling is stopped before its process ends and fails with `render_too_large`;
  the render bound grows with the page count; memory never holds more page
  images than requests in flight (a fake sender blocks and the test counts
  loaded pages).
- With a fake Vision service: a three-page PDF stores one entry with the joined
  text, page count 3, origin and engine id; a page error stores nothing; a
  transient error is retried and succeeds; a page with no text contributes an
  empty page and the document is still stored.
- The page limit: a document that does not fit is skipped without any request;
  with a limit of 1, a one-page document whose first request fails transiently
  is not retried and stores nothing; the fake counts requests and never sees
  more than the limit.
- A one-page document whose response has no text stores nothing and reports
  `empty_text`.
- A bundle file whose bytes do not match the listed SHA-256 sends no request.
- Cross-check selection: a PDF with only a fresh entry is selected for checking;
  with an imported and a fresh entry the imported one is selected.
- Selection: a PDF with imported text is not selected; `--pdf` selects it and
  the new entry sits beside the imported one.
- The rendered page count differing from the counted pages stores nothing.
- No credential value appears in logs, output or the store.
- Live, by the primary: three PDFs that have imported text are read with
  `--pdf`, and the new text is compared with the imported text (share of equal
  lines, and every differing line read against the page). The result goes into
  the research note with the page count and cost.
