package publicsummary

import (
	"encoding/json"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/contracttest"
)

func TestSummary_StatusClassAtVietnamDate(t *testing.T) {
	boundary := time.Date(2099, time.January, 2, 0, 0, 0, 0, time.UTC)
	lastSecond := time.Date(2099, time.January, 1, 16, 59, 59, 0, time.UTC)
	firstSecond := lastSecond.Add(time.Second)
	contract, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		stored   string
		from, to *time.Time
		now      time.Time
		class    string
		current  bool
	}{
		{"before the end", "in_force", nil, &boundary, lastSecond, "in_force", true},
		{"at the end", "in_force", nil, &boundary, firstSecond, "expired", false},
		{"partial at the end", "partial", nil, &boundary, firstSecond, "expired", false},
		{"before the start", "in_force", &boundary, nil, lastSecond, "not_yet", false},
		{"at the start", "in_force", &boundary, nil, firstSecond, "in_force", true},
		{"stored suspended unchanged", "suspended", nil, &boundary, firstSecond, "suspended", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, p, _, _ := handlerFixture(t)
			p.row.StatusClass, p.row.EffectiveFrom, p.row.EffectiveTo = tc.stored, tc.from, tc.to
			h.clock = func() time.Time { return tc.now }
			w := summaryCall(h, "/api/v1/public/documents/vn:vbpl:1")
			if w.Code != 200 {
				t.Fatalf("response: %d %s", w.Code, w.Body.String())
			}
			contracttest.NewCoverage(contract, []string{"/api/v1/public/documents/{key}"}).Check(t, "GET", "/api/v1/public/documents/{key}", w)
			var body struct {
				Document struct {
					Status struct {
						Class       string `json:"class"`
						Current     bool   `json:"current"`
						Basis       string `json:"basis"`
						StatusBasis string `json:"status_basis"`
					} `json:"status"`
				} `json:"document"`
			}
			if decodeErr := json.Unmarshal(w.Body.Bytes(), &body); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			status := body.Document.Status
			if status.Class != tc.class || status.Current != tc.current || status.Basis != "source_status" || status.StatusBasis != "source_status" {
				t.Fatalf("status: %+v", status)
			}
		})
	}
}
