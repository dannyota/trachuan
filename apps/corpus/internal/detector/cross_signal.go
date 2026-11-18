package detector

import "danny.vn/trachuan/apps/corpus/internal/stage/relate/textrule"

const (
	RawFieldNotNormalized          = "raw_field_not_normalized"
	RelationKindDisagreement       = "relation_kind_disagreement"
	PartialWithoutProvisionEffect  = "partial_without_provision_effect"
	UnmatchedOperativeClaim        = "unmatched_operative_claim"
	WaivedDocumentWithoutRelations = "waived_document_without_relations"

	SubjectDocument = "document"
	SubjectClaim    = "claim"
	SubjectRelation = "relation"

	DiscriminatorNone  = "none"
	DiscriminatorField = "field"

	CompletenessDocumentObserved = "document_in_release_not_baseline_served"
	CompletenessPairRead         = "source_text_fully_read_and_target_in_release"
	CompletenessClaimRead        = "source_article_read_and_every_target_resolved"
	CompletenessOCRDocument      = "ocr_document_without_extract_gap_not_baseline_served"
)

func crossSignal() []Detector {
	return []Detector{
		{
			ID: RawFieldNotNormalized, Version: 1, Semantics: 1, Subject: SubjectDocument,
			Inputs: []string{"release.document", "release.coverage_gap"}, Severity: Info,
			Fingerprint:   []string{"field", "raw_sha256", "normalized_sha256"},
			Discriminator: DiscriminatorField, Completeness: CompletenessDocumentObserved,
		},
		{
			ID: PartialWithoutProvisionEffect, Version: 1, Semantics: 1, Subject: SubjectDocument,
			Inputs: []string{"release.document", "release.relation", "release.coverage_gap"}, Severity: Info,
			Fingerprint:   []string{"status_class", "provision_target_relations"},
			Discriminator: DiscriminatorNone, Completeness: CompletenessDocumentObserved,
		},
		{
			ID: RelationKindDisagreement, Version: 1, Semantics: 1, Subject: SubjectRelation,
			Inputs:        []string{"release.document", "release.relation", "release.provision", "release.coverage_gap"},
			Severity:      Review,
			Fingerprint:   []string{"text_kind", "diagram_kinds"},
			Discriminator: DiscriminatorNone, Completeness: CompletenessPairRead,
		},
		{
			ID: UnmatchedOperativeClaim, Version: 1, Semantics: 1, Subject: SubjectClaim,
			Inputs:        []string{"release.document", "release.provision", "release.relation", "release.coverage_gap", "run.snapshot.central_issuers", "text_rules:" + textrule.RulesVersion},
			Severity:      Review,
			Fingerprint:   []string{"source_class", "kind", "sentence_sha256", "unmatched_targets"},
			Discriminator: DiscriminatorNone, Completeness: CompletenessClaimRead,
		},
		{
			ID: WaivedDocumentWithoutRelations, Version: 1, Semantics: 1, Subject: SubjectDocument,
			Inputs: []string{"release.document", "release.relation", "release.coverage_gap", "extract.text.json"}, Severity: Info,
			Fingerprint:   []string{"waiver_check_sha256", "extract_sha256"},
			Discriminator: DiscriminatorNone, Completeness: CompletenessOCRDocument,
		},
	}
}

func CrossSignalIDs() []string {
	specs := crossSignal()
	ids := make([]string, len(specs))
	for i, d := range specs {
		ids[i] = d.ID
	}
	return ids
}
