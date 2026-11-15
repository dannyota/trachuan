package eval

import (
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
)

func applyInheritance(report *evaluation.Report, stats build.TaxonomyStats, hasLLM bool) {
	if report.NotRun != nil && (stats.Inherited > 0 || hasLLM) {
		delete(report.NotRun, "taxonomy")
	}
	if stats.Inheritance.Status == "" {
		if stats.Inherited > 0 {
			report.QualityPass = false
		}
		return
	}
	report.Inheritance = stats.Inheritance
	if stats.Inherited > 0 && stats.Inheritance.Status != "pass" {
		report.QualityPass = false
	}
}
