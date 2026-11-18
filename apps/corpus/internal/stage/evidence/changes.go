package evidence

import (
	"io"
	"slices"
	"strings"
)

const (
	changeDocument       = "document"
	changeDocumentKey    = "document_key"
	changeLegal          = "document_legal"
	changeMetadata       = "document_metadata"
	changeProvenance     = "document_provenance"
	changeProvision      = "provision"
	changeClassification = "provision_classification"
	changeValidity       = "provision_validity"
	changeRelation       = "relation"

	causeMembership = "membership"
	causeCoverage   = "coverage"
	causeSource     = "source"
)

type changeInputs struct {
	current     string
	predecessor string
	members     members
	bindings    changesBindings
}

type valueChange[T any] struct {
	Before T `json:"before"`
	After  T `json:"after"`
}

func changed[T comparable](before, after T) *valueChange[T] {
	if before == after {
		return nil
	}
	return &valueChange[T]{Before: before, After: after}
}

func changedOptional(before, after *string) *valueChange[*string] {
	if compareOptional(before, after) == 0 {
		return nil
	}
	return &valueChange[*string]{Before: before, After: after}
}

type documentChange struct {
	Type       string  `json:"type"`
	Key        string  `json:"key"`
	Change     string  `json:"change"`
	Cause      string  `json:"cause"`
	ReplacedBy *string `json:"replaced_by,omitempty"`
}

type keyChange struct {
	Type   string `json:"type"`
	Key    string `json:"key"`
	Before string `json:"before"`
}

type metadataChange struct {
	Type       string              `json:"type"`
	Key        string              `json:"key"`
	NumberNorm valueChange[string] `json:"number_norm"`
}

type legalChange struct {
	Type             string                `json:"type"`
	Key              string                `json:"key"`
	StatusClass      *valueChange[string]  `json:"status_class,omitempty"`
	EffFrom          *valueChange[*string] `json:"eff_from,omitempty"`
	EffTo            *valueChange[*string] `json:"eff_to,omitempty"`
	IsCurrent        *valueChange[bool]    `json:"is_current,omitempty"`
	ValidityConflict *valueChange[bool]    `json:"validity_conflict,omitempty"`
}

type provenanceChange struct {
	Type         string               `json:"type"`
	Key          string               `json:"key"`
	TextSource   *valueChange[string] `json:"text_source,omitempty"`
	TextComplete *valueChange[bool]   `json:"text_complete,omitempty"`
	Baseline     *valueChange[bool]   `json:"baseline,omitempty"`
}

type documentChanges struct {
	documents, keys, legal, metadata, provenance []any
}

type handOvers struct {
	oldFor map[string]string
	newFor map[string]string
	named  map[string]bool
}

func pairHandOvers(before, after map[string]summaryRecord, m members) handOvers {
	h := handOvers{oldFor: map[string]string{}, newFor: map[string]string{}, named: map[string]bool{}}
	for old, replacement := range m.replacement {
		h.named[replacement] = true
		if present(before, old) && present(after, replacement) && !present(after, old) && !present(before, replacement) {
			h.oldFor[replacement] = old
			h.newFor[old] = replacement
		}
	}
	for replacement, count := range countValues(h.newFor) {
		if count > 1 {
			for old, value := range h.newFor {
				if value == replacement {
					delete(h.newFor, old)
				}
			}
			delete(h.oldFor, replacement)
		}
	}
	return h
}

func countValues(m map[string]string) map[string]int {
	counts := map[string]int{}
	for _, v := range m {
		counts[v]++
	}
	return counts
}

func present(docs map[string]summaryRecord, key string) bool {
	d, ok := docs[key]
	return ok && !d.Absent
}

func sortedKeys(docs map[string]summaryRecord) []string {
	keys := make([]string, 0, len(docs))
	for key := range docs {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func provenanceChanged(before, after summaryRecord) bool {
	return before.TextSource != after.TextSource || before.TextComplete != after.TextComplete
}

func compareDocument(out *documentChanges, key string, before, after summaryRecord) {
	if before.NumberNorm != after.NumberNorm {
		out.metadata = append(out.metadata, metadataChange{Type: changeMetadata, Key: key, NumberNorm: valueChange[string]{Before: before.NumberNorm, After: after.NumberNorm}})
	}
	legal := legalChange{Type: changeLegal, Key: key, StatusClass: changed(before.StatusClass, after.StatusClass), EffFrom: changedOptional(before.EffFrom, after.EffFrom), EffTo: changedOptional(before.EffTo, after.EffTo), IsCurrent: changed(before.IsCurrent, after.IsCurrent), ValidityConflict: changed(before.ValidityConflict, after.ValidityConflict)}
	if legal.StatusClass != nil || legal.EffFrom != nil || legal.EffTo != nil || legal.IsCurrent != nil || legal.ValidityConflict != nil {
		out.legal = append(out.legal, legal)
	}
	provenance := provenanceChange{Type: changeProvenance, Key: key, TextSource: changed(before.TextSource, after.TextSource), TextComplete: changed(before.TextComplete, after.TextComplete), Baseline: changed(before.Baseline, after.Baseline)}
	if provenance.TextSource != nil || provenance.TextComplete != nil || provenance.Baseline != nil {
		out.provenance = append(out.provenance, provenance)
	}
}

func removalCause(m members, key string) (string, *string) {
	switch m.disposition[key] {
	case "source_replaced":
		if replacement, ok := m.replacement[key]; ok {
			return causeSource, &replacement
		}
		return causeSource, nil
	case "active":
		return causeCoverage, nil
	}
	return causeMembership, nil
}

func documentRecords(before, after map[string]summaryRecord, m members, h handOvers) documentChanges {
	var out documentChanges
	for _, key := range sortedKeys(after) {
		if !present(after, key) {
			continue
		}
		if old, ok := h.oldFor[key]; ok {
			out.keys = append(out.keys, keyChange{Type: changeDocumentKey, Key: key, Before: old})
			compareDocument(&out, key, before[old], after[key])
			continue
		}
		if present(before, key) {
			compareDocument(&out, key, before[key], after[key])
			continue
		}
		cause := causeMembership
		if h.named[key] {
			cause = causeSource
		} else if _, listed := before[key]; listed {
			cause = causeCoverage
		}
		out.documents = append(out.documents, documentChange{Type: changeDocument, Key: key, Change: "added", Cause: cause})
	}
	for _, key := range sortedKeys(before) {
		if !present(before, key) || present(after, key) || h.newFor[key] != "" {
			continue
		}
		cause, replacement := removalCause(m, key)
		out.documents = append(out.documents, documentChange{Type: changeDocument, Key: key, Change: "removed", Cause: cause, ReplacedBy: replacement})
	}
	slices.SortStableFunc(out.documents, func(a, b any) int {
		return strings.Compare(a.(documentChange).Key, b.(documentChange).Key)
	})
	return out
}

func readDocuments(path string, check func(identityHeader) error) (map[string]summaryRecord, error) {
	var header identityHeader
	r, err := openSummary(path, &header)
	if err != nil {
		return nil, err
	}
	defer r.close()
	if header.Schema != summarySchema {
		return nil, refuse("%s schema %d is not %d", path, header.Schema, summarySchema)
	}
	if err = check(header); err != nil {
		return nil, err
	}
	docs := map[string]summaryRecord{}
	last := ""
	for {
		record, err := r.section(recordDocument)
		if err != nil {
			return nil, err
		}
		if record == nil {
			return docs, nil
		}
		if record.Key <= last {
			return nil, refuse("%s document %s follows %s", path, record.Key, last)
		}
		last = record.Key
		docs[record.Key] = *record
	}
}

func writeChanges(w io.Writer, in changeInputs) error {
	out := newLineWriter(w)
	baseline := in.predecessor == ""
	if baseline != (in.bindings.PredecessorIdentitySHA256 == noneValue) {
		return refuse("predecessor summary and its binding disagree")
	}
	if err := out.write(changesHeader{Schema: summarySchema, Bindings: in.bindings, Baseline: baseline}); err != nil {
		return err
	}
	if baseline {
		return out.flush()
	}
	after, err := readDocuments(in.current, func(h identityHeader) error {
		if h.Bindings != in.bindings.identityBindings {
			return refuse("identity summary does not bind this release")
		}
		return nil
	})
	if err != nil {
		return err
	}
	before, err := readDocuments(in.predecessor, func(h identityHeader) error {
		if h.Bindings.Rel != in.bindings.PredecessorRel {
			return refuse("predecessor summary binds %s, not %s", h.Bindings.Rel, in.bindings.PredecessorRel)
		}
		return nil
	})
	if err != nil {
		return err
	}
	h := pairHandOvers(before, after, in.members)
	docs := documentRecords(before, after, in.members, h)
	for _, records := range [][]any{docs.documents, docs.keys, docs.legal, docs.metadata, docs.provenance} {
		for _, record := range records {
			if err = out.write(record); err != nil {
				return err
			}
		}
	}
	plan, err := newProvisionPlan(in, before, after, h)
	if err != nil {
		return err
	}
	for _, kind := range []string{changeProvision, changeClassification, changeValidity} {
		if err = plan.pass(kind, out); err != nil {
			return err
		}
	}
	if err = writeRelationChanges(in, h, out); err != nil {
		return err
	}
	return out.flush()
}
