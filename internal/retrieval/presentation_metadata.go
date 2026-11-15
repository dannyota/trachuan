package retrieval

import (
	"slices"
	"strings"
)

func presentMetadata(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func presentDocument(doc DocumentEntry) DocumentEntry {
	doc.Number = presentMetadata(doc.Number)
	doc.Title = presentMetadata(doc.Title)
	doc.Related = slices.Clone(doc.Related)
	for i := range doc.Related {
		doc.Related[i].Number = presentMetadata(doc.Related[i].Number)
	}
	return doc
}

func presentWarnings(warnings []Warning) []Warning {
	out := slices.Clone(warnings)
	for i := range out {
		out[i].Number = presentMetadata(out[i].Number)
	}
	return out
}

func presentRelations(relations []Relation) []Relation {
	out := slices.Clone(relations)
	for i := range out {
		out[i].SourceNumber = presentMetadata(out[i].SourceNumber)
		out[i].TargetNumber = presentMetadata(out[i].TargetNumber)
	}
	return out
}

func presentUnit(unit Unit) Unit {
	unit.Warnings = presentWarnings(unit.Warnings)
	if unit.Relation != nil {
		relation := *unit.Relation
		relation.Number = presentMetadata(relation.Number)
		unit.Relation = &relation
	}
	return unit
}

func presentReadItem(item ItemResult) ItemResult {
	if item.Document != nil {
		doc := presentDocument(*item.Document)
		item.Document = &doc
	}
	item.Units = slices.Clone(item.Units)
	for i := range item.Units {
		item.Units[i] = presentUnit(item.Units[i])
	}
	return item
}
