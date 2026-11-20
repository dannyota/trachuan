package evaluation

import "testing"

func TestRevision_ToleranceConditionsOverComparableCases(t *testing.T) {
	previous := []revisionCase{
		{id: "a", r8: []int{7}, r20: []int{7}},
		{id: "b", r8: []int{1}, r20: []int{1}},
		{id: "c", r8: []int{1}, r20: []int{1}},
		{id: "d", r8: []int{1}, r20: []int{1}},
		{id: "x", r8: []int{1}, r20: []int{1}},
	}
	current := []revisionCase{
		{id: "a", r8: []int{8}, r20: []int{8}},
		{id: "b", r8: []int{1}, r20: []int{1}},
		{id: "c", r8: []int{1}, r20: []int{1}},
		{id: "d", r8: []int{1}, r20: []int{1}},
		{id: "x", r8: []int{1}, r20: []int{1}, question: "changed"},
	}
	for _, tc := range []struct {
		name      string
		mutate    func(t *testing.T, p, r *Report)
		tolerated bool
	}{
		{"conditions hold", func(*testing.T, *Report, *Report) {}, true},
		{"previous holds an extra record", func(t *testing.T, p, _ *Report) {
			extra := *developmentCaseRecord(t, p, "b", "exact", 8)
			extra.Arms = "dense"
			p.Cases = append(p.Cases, extra)
		}, false},
		{"rejections differ", func(_ *testing.T, p, _ *Report) { p.Rejections[0].CaseID = "other" }, false},
		{"previous record expects another count", func(t *testing.T, p, _ *Report) {
			developmentCaseRecord(t, p, "b", "exact", 8).Expected = 2
		}, false},
		{"previous record has another rank count", func(t *testing.T, p, _ *Report) {
			developmentCaseRecord(t, p, "b", "exact", 8).Ranks = []int{1, 0}
		}, false},
		{"falling case lost an alternative", func(t *testing.T, _, r *Report) {
			developmentCaseRecord(t, r, "a", "exact", 8).Mapped[0][0].DocumentKey = "other"
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, r := revisionPair(t, previous, current)
			tc.mutate(t, &p, &r)
			if err := finishQuality(&r, &p); err != nil {
				t.Fatal(err)
			}
			n := developmentRecord(t, r, "exact", "mrr", 8)
			if !near(n.Previous, (1.0/7+3)/4) || !near(n.Revision.Value, (1.0/8+3)/4) || !n.WithinTolerance {
				t.Fatalf("record = %+v revision %+v", n.MRRComparison, n.Revision)
			}
			if tc.tolerated != (n.Tolerance == mrrTolerance) || tc.tolerated != (n.Status == "pass") || tc.tolerated != r.QualityPass {
				t.Fatalf("tolerated %v: status %s tolerance %v quality %v", tc.tolerated, n.Status, n.Tolerance, r.QualityPass)
			}
		})
	}
}

func TestRevision_LossesOverComparableCases(t *testing.T) {
	previous := []revisionCase{
		{id: "a", r8: []int{1}, r20: []int{1}},
		{id: "b", r8: []int{2}, r20: []int{2}},
		{id: "x", r8: []int{1}, r20: []int{1}},
	}
	current := []revisionCase{
		{id: "a", r8: []int{1}, r20: []int{1}},
		{id: "b", r8: []int{2}, r20: []int{2}},
		{id: "x", r8: []int{2}, r20: []int{2}, question: "changed"},
	}
	p, r := revisionPair(t, previous, current)
	developmentCaseRecord(t, &r, "x", "exact", 8).Mapped[0][0].DocumentKey = "other"
	if err := finishQuality(&r, &p); err != nil {
		t.Fatal(err)
	}
	n := developmentRecord(t, r, "exact", "mrr", 8)
	if len(n.Losses) != 0 || n.Tolerance != mrrTolerance || n.WithinTolerance || n.Status != "pass" {
		t.Fatalf("record = %+v", n.MRRComparison)
	}
}

func TestRevision_LowestRankPerCase(t *testing.T) {
	previous := []revisionCase{
		{id: "a", r8: []int{2, 5}, r20: []int{2, 5}},
		{id: "b", r8: []int{1}, r20: []int{1}},
		{id: "x", r8: []int{1}, r20: []int{1}},
	}
	current := []revisionCase{
		{id: "a", r8: []int{5, 2}, r20: []int{5, 2}},
		{id: "b", r8: []int{1}, r20: []int{1}},
		{id: "x", r8: []int{1}, r20: []int{1}, question: "changed"},
	}
	p, r := revisionPair(t, previous, current)
	if err := finishQuality(&r, &p); err != nil {
		t.Fatal(err)
	}
	n := developmentRecord(t, r, "exact", "mrr", 8)
	if !near(n.PreviousValue, .75) || !near(n.Revision.Value, .75) || n.Status != "pass" {
		t.Fatalf("record = %+v revision %+v", n, n.Revision)
	}
}

func TestRevision_WatermarkAndStrictRule(t *testing.T) {
	previous := []revisionCase{
		{id: "a", r8: []int{1}, r20: []int{1}},
		{id: "b", r8: []int{2}, r20: []int{2}},
		{id: "x", r8: []int{1}, r20: []int{1}},
	}
	current := []revisionCase{
		{id: "a", r8: []int{1}, r20: []int{1}},
		{id: "b", r8: []int{2}, r20: []int{2}},
		{id: "x", r8: []int{1}, r20: []int{1}, question: "changed"},
	}
	for _, tc := range []struct {
		name      string
		mutate    func(p *Report)
		highWater float64
		tolerance float64
	}{
		{"strict rule compares with the recomputed value", func(p *Report) { p.Rejections[0].CaseID = "other" }, .753, 0},
		{"version one previous carries no watermark", func(p *Report) { p.Version = 1 }, .75, mrrTolerance},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, r := revisionPair(t, previous, current)
			setPreviousMRR(&p, .7, .703, nil)
			tc.mutate(&p)
			if err := finishQuality(&r, &p); err != nil {
				t.Fatal(err)
			}
			n := developmentRecord(t, r, "exact", "mrr", 8)
			if !near(n.Previous, .75) || !near(n.Revision.Value, .75) || *n.Value != .5 {
				t.Fatalf("record = %+v revision %+v", n.MRRComparison, n.Revision)
			}
			if !near(n.HighWater, tc.highWater) || n.Tolerance != tc.tolerance || n.WithinTolerance || n.Status != "pass" {
				t.Fatalf("record = %+v", n.MRRComparison)
			}
		})
	}
}
