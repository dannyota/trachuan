package mcpapi

type guideResult struct {
	Guide string `json:"guide"`
}

const frameworkGuideText = `For control frameworks use corpus:"frameworks" instead of country with corpus_status, search, read and quality_gaps. Filter search by frameworks (codes) or versions (fw:<code>:<version>). Search an exact code or alias followed by a citation for a direct lookup. Read fw:<code>:<version>#<citation> handles together; never mix law and framework handles in one read. Use mappings for edges at a control or between two versions.
A licensed version without an entitlement returns a reduced record: our title, citation, kind, status, lineage and mappings. text_withheld:true means the publisher's title, text and snippets are absent. Mapping ends always use public fields; evidence appears only when permitted. Only cite text after following next to complete:true. Pin release while reading, and restart continuations when release or access changes. access_unavailable:true means the entitlement lookup failed and this request uses an empty access set.`

const guideText = `Use Vietnamese with diacritics or an exact document number. Use taxonomy to find node codes, then search with nodes as an ordering preference, then read. Start with search; scan documents and hits, then send chosen handles together in one read call. Choose a path anchor as <doc>#<anchor> to read a whole article or chapter. A snippet is not the provision. […] marks omitted text. By default search puts documents in force today first and returns few others in non_current; for a question about a past period, send in_force_only:false. abstain:true is advisory and keeps its hits; it judges only hits, not non_current.
Before relying on a document, check status, warnings and incoming (in) pointers of kinds repeals, replaces, suspends and amends_supplements. Read in:implements documents for detailed rules. Pointer direction is as recorded by the source; do not infer an inverse legal effect. A pointer outside the release must never be replaced by a number lookup.
Follow next tokens to the last page before citing read text. Only text returned complete may be cited. Use the returned citation labels in ancestor order, followed by the document number, for example: Điều 1, khoản 2, điểm a, <number>. Never infer labels from an anchor. Include the source URL and release with a citation.
Use quality_gaps with node to list what is missing in a topic.
Use report_issue when evidence reveals wrong text, wrong validity, a broken citation or a missing document. Only structured fields are accepted: a handle, kind and the typed suggestion members for that kind. Reports go to operator review and never change the corpus automatically.`
