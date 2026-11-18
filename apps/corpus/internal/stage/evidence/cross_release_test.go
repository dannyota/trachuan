package evidence

import (
	"context"
	"sort"
)

type fakeRelease struct {
	documents  []releaseDocument
	gaps       map[string]map[string]bool
	allGaps    map[string]map[string]bool
	provisions map[int64][]releaseProvision
	relations  map[relationEnds][]int64
	pairs      []pairRelation
	anchorsErr error
	queries    int
}

func (f *fakeRelease) Documents(_ context.Context, fn func(releaseDocument) error) error {
	for _, d := range f.documents {
		if err := fn(d); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeRelease) ExtractGaps(context.Context) (map[string]map[string]bool, error) {
	return f.gaps, nil
}

func (f *fakeRelease) Articles(_ context.Context, fn func(int64, []releaseProvision) error) error {
	ids := make([]int64, 0, len(f.provisions))
	for id := range f.provisions {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		if err := fn(id, f.provisions[id]); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeRelease) Anchors(_ context.Context, id int64) ([]releaseAnchor, error) {
	f.queries++
	if f.anchorsErr != nil {
		return nil, f.anchorsErr
	}
	var out []releaseAnchor
	for _, p := range f.provisions[id] {
		out = append(out, releaseAnchor{ID: p.ID, Anchor: p.Anchor})
	}
	return out, nil
}

func (f *fakeRelease) ProvisionRelations(context.Context) (map[relationEnds][]int64, error) {
	return f.relations, nil
}

func (f *fakeRelease) PairRelations(_ context.Context, fn func(pairRelation) error) error {
	for _, p := range f.pairs {
		if err := fn(p); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeRelease) TextGaps(context.Context) (map[string]bool, error) {
	out := map[string]bool{}
	for key, kinds := range f.allGaps {
		for _, kind := range textRowGapKinds() {
			if kinds[kind] {
				out[key] = true
			}
		}
	}
	return out, nil
}
