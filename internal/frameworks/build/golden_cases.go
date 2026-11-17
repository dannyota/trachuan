package build

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"danny.vn/trachuan/internal/frameworks/businesskey"
	"danny.vn/trachuan/internal/frameworks/registry"
	"danny.vn/trachuan/internal/frameworks/relate"
)

type GoldenCase struct {
	ID           string `json:"id"`
	Question     string `json:"-"`
	Code         string `json:"code"`
	Version      string `json:"version"`
	Target       string `json:"target"`
	Access       string `json:"access,omitempty"`
	Negative     bool   `json:"negative"`
	Adjudication string `json:"adjudication"`
	Reason       string `json:"-"`
}

type caseFailure struct {
	id, operation string
	cause         error
}

func (e *caseFailure) Error() string { return fmt.Sprintf("case %q: %s", e.id, e.operation) }
func (e *caseFailure) Unwrap() error { return e.cause }

func MapGoldenCases(input io.Reader, reg *registry.Registry, controls []Control) ([]GoldenCase, error) {
	r := csv.NewReader(input)
	header, err := r.Read()
	if err != nil {
		return nil, hide("read golden header", err)
	}
	columns := map[string]int{}
	for i, name := range header {
		if _, exists := columns[name]; exists {
			return nil, fmt.Errorf("duplicate golden column")
		}
		columns[name] = i
	}
	wanted := []string{"id", "question", "framework_code", "version_label", "citation_norm", "expect_abstain", "expect_fail", "notes", "adjudication", "reason"}
	if len(columns) != len(wanted) {
		return nil, fmt.Errorf("invalid golden header")
	}
	for _, name := range wanted {
		if _, ok := columns[name]; !ok {
			return nil, fmt.Errorf("invalid golden header")
		}
	}
	registered := map[string]bool{}
	for _, v := range reg.Frameworks {
		registered[v.Code] = true
	}
	targets := map[relate.Key]string{}
	for _, c := range controls {
		key := relate.Key{Code: c.Code, Version: c.Version, Citation: c.CitationNorm}
		if _, exists := targets[key]; exists {
			return nil, fmt.Errorf("duplicate golden target")
		}
		targets[key] = "fw:" + c.Code + ":" + c.Version + "#" + businesskey.EncodeCitation(c.Citation)
	}
	validator := businesskey.New()
	var cases []GoldenCase
	seen := map[string]bool{}
	for {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			return cases, nil
		}
		if err != nil {
			return nil, hide("read golden row", err)
		}
		get := func(key string) string { return row[columns[key]] }
		c := GoldenCase{ID: get("id"), Question: get("question"), Code: get("framework_code"), Version: get("version_label"), Adjudication: get("adjudication"), Reason: get("reason")}
		if strings.TrimSpace(c.ID) == "" || strings.ContainsAny(c.ID, "\r\n") {
			return nil, fmt.Errorf("invalid golden case id")
		}
		fail := func(operation string) ([]GoldenCase, error) {
			return nil, &caseFailure{id: c.ID, operation: operation}
		}
		if seen[c.ID] {
			return fail("duplicate id")
		}
		seen[c.ID] = true
		if c.Question == "" {
			return fail("missing question")
		}
		if c.Adjudication != "accepted" && c.Adjudication != "rejected" && c.Adjudication != "out_of_release" && c.Adjudication != "unverified" {
			return fail("invalid adjudication")
		}
		c.Negative, err = strconv.ParseBool(get("expect_abstain"))
		if err != nil {
			return fail("invalid abstention label")
		}
		expectFail, err := strconv.ParseBool(get("expect_fail"))
		if err != nil {
			return fail("invalid failure label")
		}
		if expectFail && c.Adjudication == "accepted" {
			return fail("accepted case expects failure")
		}
		if !c.Negative {
			if !registered[c.Code] || c.Version == "" {
				return fail("invalid framework or version")
			}
			_, version, exists := relate.ResolveVersion(reg, c.Code, c.Version)
			c.Version = version
			var matched *registry.Framework
			for i := range reg.Frameworks {
				v := &reg.Frameworks[i]
				if exists && v.Code == c.Code && v.Version == version && v.Parser != "" {
					matched = v
					break
				}
			}
			if matched == nil {
				c.Adjudication = "out_of_release"
			} else {
				if c.Adjudication == "out_of_release" {
					return fail("out_of_release adjudication for an included version")
				}
				key := relate.Key{Code: c.Code, Version: c.Version, Citation: get("citation_norm")}
				key.Citation = relate.NormalizeCitation(reg, key)
				if !validator.Valid(key.Code, key.Version, key.Citation) || targets[key] == "" {
					return fail("unresolved citation")
				}
				c.Target, c.Access = targets[key], matched.Access
			}
		}
		cases = append(cases, c)
	}
}
