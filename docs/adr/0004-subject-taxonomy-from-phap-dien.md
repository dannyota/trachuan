# ADR 0004: Subject taxonomy seeded from Bộ pháp điển

Status: accepted 2026-11-08.

## Context

Agents need to narrow a search by subject before retrieving. VBPL classifies only at document level. Bộ pháp điển (45 chủ đề, 271 đề mục, with phần/chương/mục inside each đề mục) places every in-force article in a tree, but its public portal now requires authentication; a 2023 official offline package exists with 86,388 placed articles linked to VBPL ItemIDs.

## Decision

Levels 1 to 4 of the subject tree come from the 2023 codification package, keeping names and codes. Custom nodes are declared in a YAML file for subjects the codification lacks. Articles matching the package receive seed labels; the rest are classified by an LLM in batches (document, then chapter with all its articles) using embedding-selected candidate nodes; manual overrides win. The subject filter never hides results: search runs over the whole corpus and reports the count outside the filter.

## Consequences

Official structure with little hand work; a ground-truth set to measure the classifier. The package is from 2023, so post-2023 law depends on the LLM layer. Pháp điển data is never served to users.

## Alternatives rejected

A hand-written tree. Pure embedding zero-shot classification. A fine-tuned classifier. Hard-cut subject filtering.

## History

- 2026-11-08: accepted in the brainstorming conversation.
- 2026-11-08: after design review findings S-08 to S-11, fixed the precedence
  (manual over seed over llm), moved classification after embedding, and
  defined the import mapping and code rules in design/taxonomy.md.
