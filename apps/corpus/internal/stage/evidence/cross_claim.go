package evidence

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"danny.vn/trachuan/apps/corpus/internal/detector"
	"danny.vn/trachuan/apps/corpus/internal/source"
)

const (
	classRead       = "review"
	classIncomplete = "info"
)

type claimDetector struct{}

func (claimDetector) Spec() detector.Detector { return specOf(detector.UnmatchedOperativeClaim) }

func (claimDetector) Detect(ctx context.Context, in Inputs) (Observation, error) {
	list, err := source.Parse(in.Run.Snapshot.SourceList)
	if err != nil {
		return Observation{}, fmt.Errorf("read source list: %w", err)
	}
	return detectClaims(ctx, sqlRelease{in.Release}, list.CentralIssuers)
}

type docIdentity struct{ issuer, issueDate string }

type claimContext struct {
	issuers    map[string]string
	numbers    map[string][]int64
	identities map[int64]docIdentity
}

type claimTarget struct {
	number, anchor string
	document       int64
	ok             bool
}

type claim struct {
	document, anchor  string
	subtree           []int64
	ordinal           int
	class, kind, hash string
	severity          detector.Severity
	targets           []claimTarget
}

type claimScan struct {
	claims  []claim
	unread  map[string]bool
	blocked map[string]bool
}

func (c claim) subject() Subject {
	return Subject{Type: detector.SubjectClaim, Fields: []string{c.document, c.anchor, strconv.Itoa(c.ordinal)}}
}

func articleKey(document, anchor string) string { return document + "\x00" + anchor }

func detectClaims(ctx context.Context, rel releaseData, issuers map[string]string) (Observation, error) {
	if len(issuers) == 0 {
		return Observation{}, errors.New("run has no central issuer table")
	}
	cov := newCoverage()
	keys := map[int64]string{}
	unread := map[int64]bool{}
	cx := claimContext{issuers: issuers, numbers: map[string][]int64{}, identities: map[int64]docIdentity{}}
	err := rel.Documents(ctx, func(d releaseDocument) error {
		cov.add(d)
		keys[d.ID] = d.Key
		unread[d.ID] = neverRead(d)
		if d.NumberNorm != "" {
			cx.numbers[d.NumberNorm] = append(cx.numbers[d.NumberNorm], d.ID)
		}
		cx.identities[d.ID] = docIdentity{issuer: d.Issuer, issueDate: d.RawIssueDate}
		return nil
	})
	if err != nil {
		return Observation{}, err
	}
	gaps, err := rel.ExtractGaps(ctx)
	if err != nil {
		return Observation{}, err
	}
	scan := claimScan{unread: map[string]bool{}, blocked: map[string]bool{}}
	read := map[string]bool{}
	err = rel.Articles(ctx, func(id int64, rows []releaseProvision) error {
		key := keys[id]
		read[key] = true
		blocked, headings := gapState(gaps[key])
		class := classRead
		if blocked || unread[id] {
			class = classIncomplete
		}
		if blocked {
			scan.blocked[key] = true
		}
		found, skipped := readClaims(key, rows, blocked || headings, class, cx)
		scan.claims = append(scan.claims, found...)
		for _, anchor := range skipped {
			scan.unread[articleKey(key, anchor)] = true
		}
		return nil
	})
	if err != nil {
		return Observation{}, err
	}
	for _, key := range keys {
		if !read[key] {
			scan.blocked[key] = true
		}
	}
	for key, g := range gaps {
		if blocked, _ := gapState(g); blocked {
			scan.blocked[key] = true
		}
	}
	return resolveClaims(ctx, rel, cov, scan)
}
