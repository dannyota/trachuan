# Web voice

Decision: [ADR 0018](../adr/0018-family-visual-identity.md). Visual system:
[web visual system](web-visual.md).

Tra Chuẩn speaks like a person where it helps someone use the site, and stays
neutral and exact wherever a person or an agent might quote what it shows.

## Rules

- **Person.** Tra Chuẩn speaks as "mình" and calls the reader "bạn"; in English
  "I" and "you". Outside the maker panel, "mình" is always Tra Chuẩn; in the
  maker panel it is the owner.
- **Headlines.** One short sentence, often two parallel halves, ending with a
  full stop.
- **Facts.** Short, checkable sentences, one per line. A number appears with its
  unit and date in the same sentence and comes from data, never from copy.
- **Control.** What the reader can do is stated as theirs: "Bạn cấp quyền chỉ
  đọc và thu hồi lúc nào cũng được."
- **Actions.** Verb and object, sentence case: "Tra cứu", "Xem cách kết nối".
- **English.** Rewritten in the same voice, not translated word for word.
- **Never.** Exclamation marks, superlatives, emoji, em dashes, arrows appended
  to links, uppercase outside a stamp, invented numbers.

## Claims

Copy states only what is true today.

- No copy promises that a result answers the question or that an assistant will
  not guess. Search can abstain and can return low-confidence hits
  ([retrieval recipe](retrieval-recipe.md)), and what an assistant writes is
  outside Tra Chuẩn. Copy says what the product does: it finds passages in the
  source documents and shows them word for word, with the document number and
  status; an assistant can read and quote them.
- A stamp is the status recorded in Tra Chuẩn's dataset, with its stated basis,
  which may be the source, a derivation or a reviewed correction. Copy never
  calls it validity on a date and never lists some status terms as if they were
  the whole set. The as-of line beside a stamp appears only where a status date
  is shown.
- A link to the original exists only when the source publishes one.
- Questions are never written to disk or logs. They are held in memory caches
  for a limited time ([privacy](web.md#public-no-account)), so copy never says
  they are not stored at all.
- Counts come from `GET /api/v1/public/status` or the taxonomy response.

## Voice and typeface per field

The rule applies per content field, not per screen: one screen can hold a
speaking intro around neutral facts.

| Field                                                                                 | Voice                                            | Typeface                      |
| ------------------------------------------------------------------------------------- | ------------------------------------------------ | ----------------------------- |
| Verbatim provision text                                                               | none; quoted exactly                             | Noto Serif                    |
| Document titles, everywhere                                                           | none; quoted exactly                             | Noto Serif                    |
| Provision labels and headings                                                         | none; quoted exactly                             | Noto Serif                    |
| Document numbers                                                                      | none                                             | Roboto Mono                   |
| Validity stamps and the status basis line                                             | neutral; fixed terms                             | chrome face, stamps uppercase |
| Warning sentences                                                                     | neutral; fixed template ([web](web.md#warnings)) | chrome face                   |
| Citations and relation labels                                                         | neutral; fixed format                            | chrome face                   |
| Search quality notices ("Bằng chứng yếu")                                             | neutral                                          | chrome face                   |
| Topic names and descriptions (taxonomy data)                                          | neutral; shown as served                         | chrome face                   |
| Counts, release id and gap facts                                                      | neutral numbers and facts in a speaking frame    | chrome face, tabular numbers  |
| Report evidence on the account page                                                   | neutral; shown as stored                         | chrome face                   |
| OAuth consent, security warnings, recovery codes, re-authentication, account deletion | neutral, exact                                   | chrome face                   |
| MCP tool output and `/llms.txt` agent guidance                                        | neutral                                          | none                          |
| Privacy and terms                                                                     | neutral, third person ("Tra Chuẩn")              | chrome face                   |
| Headlines, intros, helper text, empty, idle and error states                          | may speak as "mình"                              | chrome face                   |
| Maker panel                                                                           | the owner, first person                          | chrome face                   |

## Maker panel

Home ends with the owner's introduction: a greeting, his reason for building Tra
Chuẩn in his own words, a line on what is still missing, his picture as a
circle, and pills to search, to his resume at aboutme.vn/danny and to mail
support. The picture is served from the same origin.
