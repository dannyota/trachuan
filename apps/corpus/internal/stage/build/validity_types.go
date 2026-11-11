package build

import (
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/validity"
)

type validityDocument struct {
	key, number, numberNorm, docType, issuer, issue          string
	statusCode, statusName, sourceClass, initialClass, basis string
	rawFrom, rawTo, from, to                                 string
	dateGap, consolidated                                    bool
	article                                                  *validity.Result
	textSource, anchor, quote, extractHash, fetchHash        string
}

type validityRelation struct {
	source, target, kind string
	id                   int64
}
type validityDecision struct {
	layer, version                       string
	before, after                        validityValues
	actor, date, anchor, quote, artifact string
	relationID                           int64
}
type validityOutputDocument struct {
	values  validityValues
	history []validityDecision
}
type validityReview struct {
	targetStatusCode, targetStatusName, targetRawFrom, targetRawTo string
	targetFetchHash, targetExtractHash                             string
	kind, key, number, targetKey, targetNumber, detail             string
	statusCode, statusName, rawFrom, rawTo                         string
	relationID                                                     int64
	sentences                                                      []string
	extractHash, fetchHash                                         string
	proposed                                                       *source.Correction
}
type validityCounts struct {
	basis               map[string]int
	ruleA, ruleB, ruleC int
	review              map[string]int
}
type validityResult struct {
	documents map[string]validityOutputDocument
	reviews   []validityReview
	counts    validityCounts
}
