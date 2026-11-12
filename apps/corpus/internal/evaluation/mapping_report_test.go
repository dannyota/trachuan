package evaluation

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

type mappingEntryJSON struct {
	CaseID        string `json:"case_id"`
	Cohort        string `json:"cohort"`
	CitationIndex int    `json:"citation_index"`
	Detail        string `json:"detail"`
}

type mappingMetricJSON struct {
	Name   string   `json:"name"`
	Status string   `json:"status"`
	Value  *float64 `json:"value"`
	Reason string   `json:"reason"`
	Count  int      `json:"mapping_incomplete_count"`
}

type mappingReportJSON struct {
	Mapping []mappingEntryJSON  `json:"mapping_incomplete"`
	Floors  []mappingMetricJSON `json:"floors"`
	Cohorts map[string]struct {
		Metrics       []mappingMetricJSON `json:"metrics"`
		Targets       []mappingMetricJSON `json:"targets"`
		NonRegression []mappingMetricJSON `json:"non_regression"`
	} `json:"cohorts"`
}

func mappingReportForTest(t *testing.T, issues ValidationReport) mappingReportJSON {
	t.Helper()
	date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
	r, _, err := Run(context.Background(), &recordedEngine{fakeEngine: fakeEngine{date: date}}, developmentSuite(), Options{EvaluationDate: date, Mapping: issues})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var out mappingReportJSON
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestReport_MappingIncompleteListed(t *testing.T) {
	issues := ValidationReport{
		"positive":             {{CitationIndex: 0, Kind: "mapping_incomplete", Field: "evidence[0].document_key", Detail: "evidence[0].document_key: missing"}, {CitationIndex: 0, Kind: "mapping_incomplete", Field: "alternatives[0].source_number", Detail: "alternatives[0].source_number: missing"}},
		"development-positive": {{CitationIndex: 0, Kind: "mapping_incomplete", Field: "entry", Detail: "entry: missing"}},
	}
	got := mappingReportForTest(t, issues).Mapping
	want := []mappingEntryJSON{{"development-positive", "development", 0, "entry: missing"}, {"positive", "old_product", 0, "alternatives[0].source_number: missing"}, {"positive", "old_product", 0, "evidence[0].document_key: missing"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mapping = %+v, want %+v", got, want)
	}
	if empty := mappingReportForTest(t, nil).Mapping; empty == nil || len(empty) != 0 {
		t.Fatalf("complete mapping = %+v, want empty array", empty)
	}
}

func TestReport_RejectionMappingIssueListedWithoutAffectingMetrics(t *testing.T) {
	issues := ValidationReport{"reject1": {{CitationIndex: 0, Kind: "mapping_incomplete", Field: "entry", Detail: "entry: missing"}}}
	got := mappingReportForTest(t, issues)
	want := []mappingEntryJSON{{"reject1", "old_product", 0, "entry: missing"}}
	if !reflect.DeepEqual(got.Mapping, want) {
		t.Errorf("mapping = %+v, want %+v", got.Mapping, want)
	}
	baseline := mappingReportForTest(t, nil)
	if !reflect.DeepEqual(got.Cohorts["old_product"], baseline.Cohorts["old_product"]) {
		t.Error("rejection mapping changed old product metrics or quality records")
	}
	if !reflect.DeepEqual(got.Floors, baseline.Floors) {
		t.Error("rejection mapping changed floors")
	}
}

func TestMetric_UnavailableNamesMappingReason(t *testing.T) {
	issues := ValidationReport{"positive": {{CitationIndex: 0, Kind: "mapping_incomplete", Detail: "first"}, {CitationIndex: 0, Kind: "mapping_incomplete", Detail: "second"}}}
	report := mappingReportForTest(t, issues)
	cohort := report.Cohorts["old_product"]
	for _, records := range [][]mappingMetricJSON{cohort.Metrics, cohort.Targets, cohort.NonRegression} {
		found := 0
		for _, record := range records {
			if record.Name != "recall" && record.Name != "macro_recall" && record.Name != "mrr" {
				continue
			}
			found++
			if record.Value != nil || record.Reason != "mapping_incomplete" || record.Count != 1 {
				t.Errorf("record = %+v", record)
			}
		}
		if found == 0 {
			t.Error("missing affected records")
		}
	}
	for _, floor := range report.Floors {
		if floor.Status != "pass" || floor.Reason != "" || floor.Count != 0 {
			t.Errorf("floor = %+v", floor)
		}
	}
	for _, metric := range report.Cohorts["development"].Metrics {
		if metric.Reason != "" || metric.Count != 0 {
			t.Errorf("development metric = %+v", metric)
		}
	}
}
