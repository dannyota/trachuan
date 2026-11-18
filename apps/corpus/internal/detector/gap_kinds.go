package detector

func gapKinds() []string {
	return []string{
		"ambiguous_tree_text", "article_number_out_of_range", "article_sequence", "baseline_served", "chunk_omission",
		"consolidated_status_unordered", "consolidated_status_unresolved", "correction_evidence_unavailable", "detail_id_mismatch",
		"embedding_input_bytes_exceeded", "embedding_input_too_long", "empty_text", "eval_floor_failed", "eval_rejection_failed",
		"file_format_unsupported", "file_too_large", "image_content", "invalid_ancestry", "invalid_date", "invalid_source_data",
		"invalid_validity_interval", "listing_only", "listing_unstable", "missing_attachment", "missing_commencement",
		"missing_effective_date", "multiple_commencement_sentences", "multiple_effect_articles", "named_document_ambiguous",
		"named_document_missing", "needs_ocr", "no_chunks", "no_text", "no_text_source", "no_tree", "not_found", "ocr_flagged",
		"ocr_label_flagged", "ocr_rejected", "ocr_waiver_stale", "override_target_missing", "partial_status_unexplained",
		"relation_anchor_ambiguous", "relation_article_unresolved", "relation_clause_unresolved", "relation_text_ambiguous_target",
		"relation_text_document_level", "relation_text_grammar", "relation_text_identity_conflict", "relation_text_local_number",
		"relation_text_mixed_verbs", "relation_text_multi_target", "relation_text_nested_scope", "relation_text_scope_rejected",
		"relation_text_source_incomplete", "relation_text_source_order", "relation_text_type_mismatch",
		"relation_text_unbalanced_quote", "relation_text_unlinked_target", "relation_text_unquoted_wording",
		"relation_text_unsupported_locator", "relation_text_yearless_number", "response_too_large", "scope_removed",
		"seed_article_missing", "source_identity_ambiguous", "source_provenance_missing", "source_replaced", "source_unavailable",
		"source_withdrawn_retained", "structure_uncertain", "summary_field_capped", "summary_outline_capped", "summary_row_capped",
		"table_text", "text_limit", "text_number_mismatch", "text_source_ambiguous", "tree_headings_only", "tree_headings_partial",
		"tree_text_partial", "tree_too_large", "unknown_node_kind", "unknown_relation_code", "unknown_status", "unknown_target",
		"unresolved_provision_relation", "withdrawn",
	}
}
