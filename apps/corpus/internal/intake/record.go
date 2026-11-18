package intake

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"danny.vn/trachuan/internal/retrieval"
)

const (
	RulesVersion    = "reports/1"
	MaxFileBytes    = 8 << 20
	MaxRecords      = 20000
	MaxNewPerImport = 500
)

var (
	countryPattern = regexp.MustCompile(`^[a-z]{2}$`)
	hashID         = regexp.MustCompile(`^[0-9a-f]{64}$`)
	slugID         = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

	origins = []string{"issue_report", "sc365", "eval"}
	kinds   = []string{"wrong_text", "wrong_validity", "broken_citation", "missing_document"}
	codes   = []string{"repealed_by", "amended_by", "replaced_by", "suspended_by", "wrong_number", "missing_document", "ambiguous_anchor", "empty_text", "status_conflict", "missing_source_url", "reported"}
	acting  = []string{"repealed_by", "amended_by", "replaced_by", "suspended_by"}
)

type Identity struct{ Key, Anchor string }

type Target struct{ Key, Anchor, Number string }

type Claim struct {
	Code   string
	Acting Identity
}

type Record struct {
	Origin, OriginID, Kind string
	Target                 Target
	Claim                  Claim
}

type File struct{ Records []Record }

func (f File) Len() int { return len(f.Records) }

type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string {
	const shown = 20
	problems := e.Problems
	suffix := ""
	if len(problems) > shown {
		suffix = "; and more"
		problems = problems[:shown]
	}
	return "invalid reports: " + strings.Join(problems, "; ") + suffix
}

func SourcePath(country string) (string, error) {
	if !countryPattern.MatchString(country) {
		return "", fmt.Errorf("invalid country %q", country)
	}
	return "sources/" + country + "-reports.yaml", nil
}

func (r Record) pair() [2]string { return [2]string{r.Origin, r.OriginID} }

func compare(a, b Record) int {
	if c := strings.Compare(a.Origin, b.Origin); c != 0 {
		return c
	}
	return strings.Compare(a.OriginID, b.OriginID)
}

func (r Record) validate(country string) []string {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	switch {
	case !slices.Contains(origins, r.Origin):
		add("origin %q is not registered", r.Origin)
	case r.Origin == "issue_report" && !hashID.MatchString(r.OriginID):
		add("origin_id of an issue_report must be 64 lower-case hexadecimal characters")
	case r.Origin != "issue_report" && !slugID.MatchString(r.OriginID):
		add("origin_id must match %s", slugID)
	}
	if !slices.Contains(kinds, r.Kind) {
		add("kind %q is not allowed", r.Kind)
	}
	if !slices.Contains(codes, r.Claim.Code) {
		add("claim code %q is not allowed", r.Claim.Code)
	}
	if (r.Kind == "missing_document") != (r.Target.Number != "") {
		add("a number target belongs to kind missing_document and only to it")
	}
	if r.Target.Number != "" {
		if r.Target.Key != "" || r.Target.Anchor != "" {
			add("a number target has no key or anchor")
		}
		if !validNumber(r.Target.Number) {
			add("target number %q is not a normalized document number", r.Target.Number)
		}
	} else {
		problems = append(problems, identityProblems("target", country, r.Target.Key, r.Target.Anchor)...)
	}
	if slices.Contains(acting, r.Claim.Code) {
		problems = append(problems, identityProblems("claim.acting", country, r.Claim.Acting.Key, r.Claim.Acting.Anchor)...)
	} else if r.Claim.Acting != (Identity{}) {
		add("claim code %q takes no acting identity", r.Claim.Code)
	}
	return problems
}

func identityProblems(at, country, key, anchor string) []string {
	var problems []string
	if !keyPattern(country).MatchString(key) {
		problems = append(problems, at+".key must match "+keyPattern(country).String())
	}
	if anchor != "" && !validAnchor(anchor) {
		problems = append(problems, fmt.Sprintf("%s.anchor %q is invalid", at, anchor))
	}
	return problems
}

var keyPatterns sync.Map

func keyPattern(country string) *regexp.Regexp {
	if p, ok := keyPatterns.Load(country); ok {
		return p.(*regexp.Regexp)
	}
	p := regexp.MustCompile(`^` + regexp.QuoteMeta(country) + `:(vbpl|chinhphu):[0-9A-Za-z_-]{1,64}$`)
	keyPatterns.Store(country, p)
	return p
}

func noncharacter(c rune) bool { return (c >= 0xFDD0 && c <= 0xFDEF) || c&0xFFFE == 0xFFFE }

func validNumber(number string) bool {
	if utf8.RuneCountInString(number) > 128 || number != retrieval.NormalizeNumber(number) {
		return false
	}
	parts := strings.SplitN(number, "/", 3)
	if len(parts) != 3 || !digits(parts[0], 1, 4) || !digits(parts[1], 4, 4) {
		return false
	}
	segments := strings.Split(strings.ReplaceAll(parts[2], "/", "-"), "-")
	if len(segments) > 4 {
		return false
	}
	for _, segment := range segments {
		if segment == "" || len(segment) > 32 || !validSegment(segment) {
			return false
		}
	}
	return true
}

func validSegment(segment string) bool {
	if strings.HasPrefix(segment, " ") || strings.HasSuffix(segment, " ") || strings.Contains(segment, "  ") {
		return false
	}
	for _, c := range segment {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '&' && c != '.' && c != '_' && c != ' ' || noncharacter(c) {
			return false
		}
	}
	return true
}

func digits(value string, low, high int) bool {
	if len(value) < low || len(value) > high {
		return false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func validAnchor(anchor string) bool {
	if utf8.RuneCountInString(anchor) > 64 {
		return false
	}
	for _, component := range strings.Split(anchor, "/") {
		kind, token, found := strings.Cut(component, "-")
		if !found || kind == "" || token == "" || strings.ToLower(component) != component {
			return false
		}
		for _, c := range kind {
			if c < 'a' || c > 'z' {
				return false
			}
		}
		for _, c := range token {
			if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '-' && c != '.' || noncharacter(c) {
				return false
			}
		}
	}
	return true
}
