package mcpapi

type guideResult struct {
	Guide string `json:"guide"`
}

const guideText = `Use Vietnamese with diacritics or an exact document number. Start with search; scan documents and hits, then send chosen handles together in one read call. Choose a path anchor as <doc>#<anchor> to read a whole article or chapter. A snippet is not the provision. […] marks omitted text.
Before relying on a document, check status, warnings and incoming (in) pointers of kinds repeals, replaces, suspends and amends_supplements. Read in:implements documents for detailed rules. Pointer direction is as recorded by the source; do not infer an inverse legal effect. A pointer outside the release must never be replaced by a number lookup.
Follow next tokens to the last page before citing read text. Only text returned complete may be cited. Use the returned citation labels in ancestor order, followed by the document number, for example: Điều 1, khoản 2, điểm a, <number>. Never infer labels from an anchor. Include the source URL and release with a citation.
Use report_issue when evidence reveals wrong text, wrong validity, a broken citation or a missing document. Only structured fields are accepted: a handle, kind and the typed suggestion members for that kind. Reports go to operator review and never change the corpus automatically.`
