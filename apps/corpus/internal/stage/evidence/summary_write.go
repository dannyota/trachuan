package evidence

import (
	"context"
	"io"
	"slices"
	"strings"
)

type summarySource interface {
	SummaryDocuments(context.Context) ([]summaryDocument, error)
	SummaryNodes(context.Context) (map[int64]taxonomyNode, error)
	SummaryProvisions(context.Context, func(string, []summaryProvision) error) error
	SummaryRelations(context.Context, func(summaryRelation) error) error
}

type members struct {
	active      map[string]bool
	disposition map[string]string
	replacement map[string]string
}

func writeIdentity(ctx context.Context, w io.Writer, src summarySource, m members, b identityBindings) error {
	out := newLineWriter(w)
	if err := out.write(identityHeader{Schema: summarySchema, Bindings: b}); err != nil {
		return err
	}
	present, err := writeDocuments(ctx, out, src, m)
	if err != nil {
		return err
	}
	nodes, err := src.SummaryNodes(ctx)
	if err != nil {
		return err
	}
	last := ""
	err = src.SummaryProvisions(ctx, func(key string, rows []summaryProvision) error {
		if key <= last {
			return refuse("provisions of %s follow %s", key, last)
		}
		if !present[key] {
			return refuse("provisions of %s have no document line", key)
		}
		last = key
		lines, lineErr := provisionLines(key, rows, nodes)
		if lineErr != nil {
			return lineErr
		}
		for _, line := range lines {
			if lineErr = out.write(line); lineErr != nil {
				return lineErr
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	var relations []summaryRelation
	if err = src.SummaryRelations(ctx, func(r summaryRelation) error {
		relations = append(relations, r)
		return nil
	}); err != nil {
		return err
	}
	sorted, err := relationLines(relations)
	if err != nil {
		return err
	}
	for _, r := range sorted {
		if err = out.write(r.line); err != nil {
			return err
		}
	}
	return out.flush()
}

func writeDocuments(ctx context.Context, out *lineWriter, src summarySource, m members) (map[string]bool, error) {
	docs, err := src.SummaryDocuments(ctx)
	if err != nil {
		return nil, err
	}
	present := make(map[string]bool, len(docs))
	for i, d := range docs {
		if i > 0 && docs[i-1].Key >= d.Key {
			return nil, refuse("documents are not in strict key order at %s", d.Key)
		}
		present[d.Key] = true
	}
	var absent []string
	for key := range m.active {
		if !present[key] {
			absent = append(absent, key)
		}
	}
	slices.Sort(absent)
	i, j := 0, 0
	for i < len(docs) || j < len(absent) {
		if j == len(absent) || (i < len(docs) && strings.Compare(docs[i].Key, absent[j]) < 0) {
			err = out.write(docs[i].line())
			i++
		} else {
			err = out.write(absentLine{Type: recordDocument, Key: absent[j], Absent: true})
			j++
		}
		if err != nil {
			return nil, err
		}
	}
	return present, nil
}
