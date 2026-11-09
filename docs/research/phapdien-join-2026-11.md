# Bộ pháp điển join coverage (2026-11-09)

Throwaway spike `spikes/phapdien-join/` parsed the 2023 Bộ pháp điển offline
package and joined every placed article to banhmi's Vietnam corpus
(`banhmi-v049.dump`, restored locally). It answers how much of the corpus the
package can label as classification seeds. Not authority; the design owns the
rules in [taxonomy](../design/taxonomy.md).

## Inputs

- Package: `BoPhapDienDienTu.zip`, sha256
  `dc2fc7d14e208b551e3e7667e911dcfd24d8f246a7e24d3d24ad7507cd4b5ff5`, from
  `github.com/definev/phapdien` `data/`, files dated 2023-11-21.
- Corpus: 3,974 documents, 81,888 `dieu` sections, PostgreSQL 18.6,
  pgvector 0.8.7.
- Corpus tables used: `silver.document_alias(source, external_id, document_id)`
  with `source = 'vbpl'` and numeric ItemIDs in `external_id` (3,773 numeric,
  136 UUID, 1 other); `silver.document(id, doc_number, is_consolidated,
  index_class)`; `silver.document_section(document_id, kind, citation_path)`
  with articles at `kind = 'dieu'` and `citation_path` ending in `dieu-<n>`;
  `silver.document_relation(relation_type = 'consolidates')` to
  `silver.doc_ref(ref_key 'vbpl:<ItemID>', label, document_id)`.

## Join rule

Document: VBPL ItemID from the article's source link, else the document number
in the source note (for example `57/2020/QH14`), compared case and space
folded against non-consolidated documents, else a consolidation relation whose
reference names the ItemID or number. Article: the number after `Dieu_` in the
link anchor, else the first `(Điều <n>` in the note.

## Package facts

| Item                                   | Count  |
| -------------------------------------- | ------ |
| chủ đề                                 | 45     |
| đề mục entries in `jdDeMuc`            | 293    |
| đề mục with no placed article          | 60     |
| đề mục marked "loại bỏ"                | 11     |
| (chủ đề, đề mục STT) pairs used twice  | 8      |
| tree nodes in `jdAllTree`              | 90,316 |
| of which Điều / Chương / Mục / Tiểu mục / Phần | 86,389 / 1,887 / 1,644 / 362 / 34 |
| placed articles parsed                 | 86,388 |
| articles with a VBPL ItemID link       | 84,981 |
| distinct ItemIDs (first link per article) | 5,682 |
| distinct MAPC codes                    | 86,307 |
| distinct (đề mục, label) pairs         | 85,655 |

The 2026-11-08 note counted 6,523 distinct ItemIDs over every link in the
files, including secondary links inside notes; the first link per article is
the one that identifies its source.

## Package articles by outcome

| Outcome                            | Method        | Articles | %    |
| ---------------------------------- | ------------- | -------- | ---- |
| document not in corpus             |               | 62,637   | 72.5 |
| matched provision                  | ItemID        | 21,872   | 25.3 |
| matched provision                  | doc number    | 1,276    | 1.5  |
| document matched, article missing  | ItemID        | 537      | 0.6  |
| document matched, article missing  | doc number    | 56       | 0.1  |
| consolidated only                  | consolidation | 10       | 0.0  |

"Not in corpus" is expected: the package covers all central law; the corpus
holds banking, finance and related technology law.

## Corpus coverage by seeds

| Corpus articles (Điều) | Total  | Seeded | %    |
| ---------------------- | ------ | ------ | ---- |
| primary                | 32,788 | 8,297  | 25.3 |
| relation context       | 40,137 | 14,727 | 36.7 |
| consolidated texts     | 8,963  | 10     | 0.1  |

| Corpus documents with a `dieu` section | Total | With a seed | %    |
| -------------------------------------- | ----- | ----------- | ---- |
| primary                                | 2,294 | 520         | 22.7 |
| relation context                       | 1,034 | 292         | 28.2 |
| consolidated texts                     | 328   | 0           | 0.0  |

Banking core: of the 5,123 package articles under chủ đề 22 (Ngân hàng, tiền
tệ), 4,695 (91.6%) join to a corpus article, 142 find the document but not
the article, 276 have no document. Chủ đề 34 joins 1,336 of 4,853; chủ đề 1
(An ninh quốc gia, including An ninh mạng) joins 459 of 1,797.

Seeded primary articles by chủ đề of the seed: 22 Ngân hàng, tiền tệ 4,305;
28 Tài sản công 850; 3 Bưu chính, viễn thông 776; 34 Thương mại, đầu tư,
chứng khoán 562; 39 Trật tự, an toàn xã hội 504; 19 Khoa học, công nghệ 170.
Only 57 corpus articles receive seeds from two đề mục; the package is almost
single-label.

## Findings

1. Seeds label one primary article in four. The LLM layer carries about 75%
   of primary articles, mostly circulars and decisions issued after 2023-11 or
   never codified. Classification cost and the held-out evaluation in the
   design must assume that share.
2. Consolidated texts can inherit seeds: 4,648 of 8,963 consolidated articles
   (51.9%) have a same-numbered seeded article in a document they consolidate
   (`consolidates` relation). Amended articles may have changed subject, so
   inheritance needs its own confidence and a check in the held-out set.
3. The tree has a fifth structural level, Tiểu mục (362 nodes), that the
   import mapping did not cover.
4. MAPC is not a unique key (81 repeats) and (đề mục, label) repeats 733
   times, mostly the same article listed twice. Articles need a load sequence
   key; tree nodes keep their GUIDs.
5. 60 đề mục have no placed article and 8 đề mục share a (chủ đề, STT) number
   with another entry, so STT-derived codes collide.
6. 593 articles find their document but not the article number: amended or
   renumbered articles. They are gaps, not seeds.

## Effect on the design

`taxonomy.md` import mapping amended for findings 3 and 5. Findings 1, 2 and 4
are inputs to Plan 1's `classify` stage and are recorded in the backlog.
