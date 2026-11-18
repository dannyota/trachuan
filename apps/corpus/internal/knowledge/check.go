package knowledge

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

const (
	OutcomeApplied   = "applied"
	OutcomeRedundant = "redundant"
	OutcomeStale     = "stale"
)

const (
	ReasonDocumentAbsent = "document_absent"
	ReasonBaseline       = "baseline"
	ReasonIncomplete     = "incomplete"
	ReasonAnchorAbsent   = "anchor_absent"
	ReasonAmbiguous      = "ambiguous"
	ReasonContract       = "contract"
	ReasonNumberNorm     = "number_norm"
	ReasonTextSource     = "text_source"
	ReasonText           = "text"
	ReasonHeading        = "heading"
	ReasonAncestors      = "ancestors"
)

type Record struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	TargetKey string `json:"target_key"`
	Outcome   string `json:"outcome"`
	Reason    string `json:"reason,omitempty"`
}

type Row struct {
	Ordinal                            int
	Kind, Label, Evidence              string
	SourceDocumentID, TargetDocumentID int64
	SourceKey, TargetKey               string
	SourceNumber, TargetNumber         string
	SourceProvisionID                  *int64
	TargetProvisionID                  *int64
	SourceNodeID, TargetNodeID         *string
}

type Report struct {
	Records  []Record
	Rows     []Row
	Problems []string
}

func (r Report) Err() error {
	if len(r.Problems) == 0 {
		return nil
	}
	return &ValidationError{Problems: r.Problems}
}

type effectRowKey struct {
	sourceProvision, targetDocument int64
	targetNode                      string
}

type checker struct {
	reader       Reader
	documents    map[string]Document
	baseline     map[string]bool
	trees        map[int64]*tree
	effectRows   map[effectRowKey][]string
	documentRows map[[2]int64][]string
}

type side struct {
	key, anchor, numberNorm string
	want                    ProvisionDepends
	named                   bool
	doc                     Document
	tree                    *tree
	node                    *Provision
	text                    string
}

type verdict struct {
	outcome, reason, problem string
	row                      *Row
}

func stale(reason string) verdict { return verdict{outcome: OutcomeStale, reason: reason} }

func Check(ctx context.Context, reader Reader, file File) (Report, error) {
	report := Report{Records: []Record{}}
	if len(file.Claims) == 0 {
		return report, nil
	}
	keys := make([]string, 0, 2*len(file.Claims))
	for _, c := range file.Claims {
		keys = append(keys, c.ActingKey, c.TargetKey)
	}
	slices.Sort(keys)
	checks, err := newChecker(ctx, reader, slices.Compact(keys))
	if err != nil {
		return Report{}, err
	}
	if err = checks.loadRows(ctx, file.Claims); err != nil {
		return Report{}, err
	}
	for _, c := range file.Claims {
		v, err := checks.claim(ctx, c)
		if err != nil {
			return Report{}, fmt.Errorf("check %s: %w", c.ID, err)
		}
		if v.problem != "" {
			report.Problems = append(report.Problems, c.ID+": "+v.problem)
			continue
		}
		report.Records = append(report.Records, Record{ID: c.ID, Kind: c.Kind, TargetKey: c.TargetKey, Outcome: v.outcome, Reason: v.reason})
		if v.row != nil {
			report.Rows = append(report.Rows, *v.row)
		}
	}
	return report, nil
}

func Depends(ctx context.Context, reader Reader, key, anchor string) (ProvisionDepends, error) {
	if strings.Contains(anchor, "~") {
		return ProvisionDepends{}, fmt.Errorf("%s %s: a ~k component is refused", key, anchor)
	}
	checks, err := newChecker(ctx, reader, []string{key})
	if err != nil {
		return ProvisionDepends{}, err
	}
	s := &side{key: key, anchor: anchor, named: true}
	sides := []*side{s}
	reason := checks.attach(sides)
	if reason == "" {
		if reason, err = checks.locate(ctx, sides); err != nil {
			return ProvisionDepends{}, err
		}
	}
	if reason != "" {
		return ProvisionDepends{}, fmt.Errorf("%s %s: refused: %s", key, anchor, reason)
	}
	return checks.actual(ctx, s)
}

func newChecker(ctx context.Context, reader Reader, keys []string) (*checker, error) {
	documents, err := reader.Documents(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("read knowledge documents: %w", err)
	}
	baseline, err := reader.BaselineServed(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("read knowledge baseline gaps: %w", err)
	}
	c := &checker{reader: reader, documents: make(map[string]Document, len(documents)), baseline: make(map[string]bool, len(baseline)), trees: map[int64]*tree{}, effectRows: map[effectRowKey][]string{}, documentRows: map[[2]int64][]string{}}
	for _, d := range documents {
		c.documents[d.Key] = d
	}
	for _, key := range baseline {
		c.baseline[key] = true
	}
	return c, nil
}

func (c *checker) loadRows(ctx context.Context, claims []Claim) error {
	seen := map[int64]bool{}
	ids := []int64{}
	for _, claim := range claims {
		if d, ok := c.documents[claim.ActingKey]; ok && !seen[d.ID] {
			seen[d.ID] = true
			ids = append(ids, d.ID)
		}
	}
	rows, err := c.reader.RelationRows(ctx, ids)
	if err != nil {
		return fmt.Errorf("read knowledge relation rows: %w", err)
	}
	for _, row := range rows {
		if row.Method == MethodReviewed {
			continue
		}
		if row.SourceProvisionID != nil && row.TargetNodeID != nil {
			key := effectRowKey{*row.SourceProvisionID, row.TargetDocumentID, *row.TargetNodeID}
			c.effectRows[key] = append(c.effectRows[key], row.Kind)
		}
		if row.SourceProvisionID == nil && row.TargetProvisionID == nil {
			key := [2]int64{row.SourceDocumentID, row.TargetDocumentID}
			c.documentRows[key] = append(c.documentRows[key], row.Kind)
		}
	}
	return nil
}

func (c *checker) claim(ctx context.Context, claim Claim) (verdict, error) {
	acting := &side{key: claim.ActingKey, anchor: claim.ActingAnchor, numberNorm: claim.ActingDepends.NumberNorm, want: claim.ActingDepends, named: true}
	target := &side{key: claim.TargetKey, anchor: claim.TargetAnchor, numberNorm: claim.TargetNumberNorm, want: claim.TargetDepends, named: claim.Kind == KindEffect}
	sides := []*side{acting, target}
	reason, err := c.resolve(ctx, sides)
	if err != nil {
		return verdict{}, err
	}
	if reason != "" {
		return stale(reason), nil
	}
	if claim.Contract != ContractVersion {
		return stale(ReasonContract), nil
	}
	for _, s := range sides {
		if s.doc.NumberNorm != s.numberNorm {
			return stale(ReasonNumberNorm), nil
		}
	}
	reason, err = c.compare(ctx, sides)
	if err != nil {
		return verdict{}, err
	}
	if reason != "" {
		return stale(reason), nil
	}
	if !strings.Contains(acting.text, claim.Quote) {
		return verdict{problem: fmt.Sprintf("quote is not in the text of %s %s", claim.ActingKey, claim.ActingAnchor)}, nil
	}
	return c.decide(claim, acting, target), nil
}

func (c *checker) resolve(ctx context.Context, sides []*side) (string, error) {
	if reason := c.attach(sides); reason != "" {
		return reason, nil
	}
	for _, s := range sides {
		if c.baseline[s.key] {
			return ReasonBaseline, nil
		}
	}
	for _, s := range sides {
		if s.named && !s.doc.TextComplete {
			return ReasonIncomplete, nil
		}
	}
	return c.locate(ctx, sides)
}

func (c *checker) attach(sides []*side) string {
	for _, s := range sides {
		doc, ok := c.documents[s.key]
		if !ok {
			return ReasonDocumentAbsent
		}
		s.doc = doc
	}
	return ""
}

func (c *checker) locate(ctx context.Context, sides []*side) (string, error) {
	for _, s := range sides {
		if !s.named {
			continue
		}
		t, err := c.tree(ctx, s.doc.ID)
		if err != nil {
			return "", err
		}
		s.tree, s.node = t, t.byAnchor[s.anchor]
		if s.node == nil {
			return ReasonAnchorAbsent, nil
		}
	}
	for _, s := range sides {
		if s.named && s.tree.ambiguous(s.node) {
			return ReasonAmbiguous, nil
		}
	}
	return "", nil
}

func (c *checker) tree(ctx context.Context, documentID int64) (*tree, error) {
	if t, ok := c.trees[documentID]; ok {
		return t, nil
	}
	provisions, err := c.reader.Provisions(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("read knowledge provisions: %w", err)
	}
	t := newTree(provisions)
	c.trees[documentID] = t
	return t, nil
}

func (c *checker) actual(ctx context.Context, s *side) (ProvisionDepends, error) {
	text, err := c.reader.ProvisionText(ctx, s.node.ID)
	if err != nil {
		return ProvisionDepends{}, fmt.Errorf("read knowledge provision text: %w", err)
	}
	s.text = text
	return ProvisionDepends{NumberNorm: s.doc.NumberNorm, TextSource: s.doc.TextSource, TextSHA256: Hash(text), HeadingSHA256: Hash(s.node.Heading), Ancestors: s.tree.ancestors(s.node)}, nil
}

func (c *checker) compare(ctx context.Context, sides []*side) (string, error) {
	named := make([]*side, 0, len(sides))
	got := map[*side]ProvisionDepends{}
	for _, s := range sides {
		if !s.named {
			continue
		}
		actual, err := c.actual(ctx, s)
		if err != nil {
			return "", err
		}
		named = append(named, s)
		got[s] = actual
	}
	for _, field := range []struct {
		reason  string
		differs func(want, have ProvisionDepends) bool
	}{
		{ReasonTextSource, func(w, h ProvisionDepends) bool { return w.TextSource != h.TextSource }},
		{ReasonText, func(w, h ProvisionDepends) bool { return w.TextSHA256 != h.TextSHA256 }},
		{ReasonHeading, func(w, h ProvisionDepends) bool { return w.HeadingSHA256 != h.HeadingSHA256 }},
		{ReasonAncestors, func(w, h ProvisionDepends) bool { return !slices.Equal(w.Ancestors, h.Ancestors) }},
	} {
		for _, s := range named {
			if field.differs(s.want, got[s]) {
				return field.reason, nil
			}
		}
	}
	return "", nil
}

func (c *checker) decide(claim Claim, acting, target *side) verdict {
	row := Row{Ordinal: claim.Position, Kind: claim.RelationKind, Label: rawLabel(claim.RelationKind), Evidence: claim.Quote, SourceDocumentID: acting.doc.ID, TargetDocumentID: target.doc.ID, SourceKey: acting.key, TargetKey: target.key, SourceNumber: acting.doc.Number, TargetNumber: target.doc.Number}
	var kinds []string
	if claim.Kind == KindEffect {
		kinds = c.effectRows[effectRowKey{acting.node.ID, target.doc.ID, target.anchor}]
		sourceNode, targetNode := acting.node.NodeID, target.anchor
		row.SourceProvisionID, row.TargetProvisionID = &acting.node.ID, &target.node.ID
		row.SourceNodeID, row.TargetNodeID = &sourceNode, &targetNode
	} else {
		kinds = c.documentRows[[2]int64{acting.doc.ID, target.doc.ID}]
	}
	if slices.Contains(kinds, claim.RelationKind) {
		return verdict{outcome: OutcomeRedundant}
	}
	if claim.Kind == KindEffect && len(kinds) > 0 {
		others := slices.Sorted(slices.Values(kinds))
		return verdict{problem: fmt.Sprintf("a matching row has kind %s", strings.Join(slices.Compact(others), ", "))}
	}
	return verdict{outcome: OutcomeApplied, row: &row}
}
