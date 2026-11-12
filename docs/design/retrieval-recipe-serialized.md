# Serialized retrieval recipe and checks

This note owns the serialized form of the retrieval recipe and its checks; it
was split from [retrieval recipe](retrieval-recipe.md).

`retrieval_recipe` contains `version:2` (recipe 2; version 1 had cap 3 and no
tail), `dense_depth`, `lexical_depth`,
`ann:{pool_min,pool_multiplier,ef_max,iterative_scan,plan_cache_mode, max_scan_tuples,scan_mem_multiplier,fallback}`,
`rrf:{k,dense_weight, lexical_weight,routed_lexical_weight}`, `ties:"score-id"`,
`routing:"vn-number/1"`, `group:"nearest-khoan"`, `reducer:"max"`,
`hydrate_multiplier`, `document_cap`, `non_current_tail`,
`backfill:"demoted-order"`, `family_collapse`,
`context_scope:"number-routed/1"`, `validity:"document-dates/3"`,
`default_in_force_only:true`, `query_gate:"vn-marks/1"`,
`evidence:"source-spans/1"`, `evidence_rune_cap`, `relation_cap`,
`relation_evidence_rune_cap`,
`abstention:{statistic:"first-group-best-member-cosine",threshold, status:"provisional-old-product"|"old-product+development",calibration_sha256}`.
Hash compact fixed-field JSON. Serving does not accept arbitrary recipe
overrides.

Any status or date eligibility change bumps `validity`, and any class
eligibility change bumps `context_scope`, even if the response shape is
unchanged. Version 2 records exclusive `eff_to` and the consolidated-status
eligibility rule above. Rebuild and re-evaluate under the new recipe; do not
reuse version 1 eligibility caches or relabel an old release.

Required checks: diacritics/rejection message, document-number boundaries,
ambiguous/unknown references, raw dense bytes, both validity modes, missing
dates, lexical-only top hit cosine, equal-score ties, roll-up and cap/backfill,
bounded evidence, expired separation, exact plan, ANN loss/fallback and pooled
connection settings restored after errors. First-party algorithm references:
[RRF](https://cormack.uwaterloo.ca/cormacksigir09-rrf.pdf) and
[pgvector indexing](https://github.com/pgvector/pgvector#indexing), cited in the
local study; no new external measurements or optimality claim.
