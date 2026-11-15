package parse

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	parameterPattern      = regexp.MustCompile(`\{\{\s*insert:\s*param,\s*([^}\s]+)\s*\}\}`)
	oscalControlIDPattern = regexp.MustCompile(`^[a-z]{2}-[0-9]+(?:\.[0-9]+)*$`)
)

type property struct {
	Name  string `json:"name"`
	Class string `json:"class"`
	Value string `json:"value"`
}
type oscalParam struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Select *struct {
		HowMany string   `json:"how-many"`
		Choice  []string `json:"choice"`
	} `json:"select"`
}
type oscalPart struct {
	Prose string      `json:"prose"`
	Parts []oscalPart `json:"parts"`
}
type oscalLink struct {
	Rel  string `json:"rel"`
	Href string `json:"href"`
}
type oscalControl struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	Props    []property     `json:"props"`
	Params   []oscalParam   `json:"params"`
	Parts    []oscalPart    `json:"parts"`
	Links    []oscalLink    `json:"links"`
	Controls []oscalControl `json:"controls"`
}
type oscalGroup struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	Props    []property     `json:"props"`
	Params   []oscalParam   `json:"params"`
	Controls []oscalControl `json:"controls"`
}

func label(id string, props []property) string {
	first := ""
	found := false
	for _, p := range props {
		if p.Name != "label" {
			continue
		}
		if !found {
			first = p.Value
			found = true
		}
		if p.Class != "zero-padded" {
			return p.Value
		}
	}
	if found {
		return first
	}
	return strings.ToUpper(id)
}
func prose(parts []oscalPart, params map[string]oscalParam) string {
	var out []string
	for _, p := range parts {
		if p.Prose != "" {
			out = append(out, renderParams(p.Prose, params))
		}
		if child := prose(p.Parts, params); child != "" {
			out = append(out, child)
		}
	}
	return strings.Join(out, "\n")
}
func renderParams(value string, params map[string]oscalParam) string {
	return parameterPattern.ReplaceAllStringFunc(value, func(match string) string {
		id := parameterPattern.FindStringSubmatch(match)[1]
		param, ok := params[id]
		if !ok {
			return match
		}
		if param.Select != nil && len(param.Select.Choice) > 0 {
			label := "Selection"
			if param.Select.HowMany == "one-or-more" {
				label = "Selection (one or more)"
			}
			choices := strings.Join(param.Select.Choice, "; ")
			if param.Label != "" {
				choices = param.Label + ": " + choices
			}
			return "[" + label + ": " + choices + "]"
		}
		if param.Label != "" {
			return "[Assignment: " + param.Label + "]"
		}
		return match
	})
}

func withParams(parent map[string]oscalParam, additions []oscalParam) map[string]oscalParam {
	merged := make(map[string]oscalParam, len(parent)+len(additions))
	for id, param := range parent {
		merged[id] = param
	}
	for _, param := range additions {
		merged[param.ID] = param
	}
	return merged
}

func parseOSCAL(data []byte, code, version string) ([]Node, error) {
	var doc struct {
		Catalog struct {
			Groups []oscalGroup `json:"groups"`
			Params []oscalParam `json:"params"`
		} `json:"catalog"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("invalid catalog JSON")
	}
	if doc.Catalog.Groups == nil {
		return nil, fmt.Errorf("missing catalog groups")
	}
	ids := map[string]string{}
	var index func([]oscalControl)
	index = func(cs []oscalControl) {
		for _, c := range cs {
			ids[c.ID] = label(c.ID, c.Props)
			index(c.Controls)
		}
	}
	for _, g := range doc.Catalog.Groups {
		index(g.Controls)
	}
	var nodes []Node
	var walk func(oscalControl, int, string, map[string]oscalParam) error
	walk = func(c oscalControl, parent int, kind string, parentParams map[string]oscalParam) error {
		params := withParams(parentParams, c.Params)
		n := Node{Citation: label(c.ID, c.Props), Kind: kind, Parent: parent, Ordinal: len(nodes), PublisherTitle: c.Title, Body: prose(c.Parts, params), Status: "active"}
		for _, p := range c.Props {
			if p.Name == "status" && p.Value == "withdrawn" {
				n.Status = "withdrawn"
			}
		}
		for _, l := range c.Links {
			if l.Rel != "related" && l.Rel != "moved-to" && l.Rel != "incorporated-into" {
				continue
			}
			if !strings.HasPrefix(l.Href, "#") {
				continue
			}
			id := strings.TrimPrefix(l.Href, "#")
			target, ok := ids[id]
			if !ok && !oscalControlIDPattern.MatchString(id) {
				continue
			}
			if !ok {
				target = strings.ToUpper(id)
			}
			n.Mappings = append(n.Mappings, Mapping{TargetFramework: code, TargetVersion: version, TargetCitation: target, Relation: l.Rel, Unresolved: !ok})
		}
		idx := len(nodes)
		nodes = append(nodes, n)
		for _, child := range c.Controls {
			if err := walk(child, idx, "enhancement", params); err != nil {
				return err
			}
		}
		return nil
	}
	catalogParams := withParams(nil, doc.Catalog.Params)
	var collect func([]oscalControl)
	collect = func(controls []oscalControl) {
		for _, control := range controls {
			for _, param := range control.Params {
				catalogParams[param.ID] = param
			}
			collect(control.Controls)
		}
	}
	for _, group := range doc.Catalog.Groups {
		for _, param := range group.Params {
			catalogParams[param.ID] = param
		}
		collect(group.Controls)
	}
	for _, g := range doc.Catalog.Groups {
		groupParams := withParams(catalogParams, g.Params)
		idx := len(nodes)
		nodes = append(nodes, Node{Citation: label(g.ID, g.Props), Kind: "family", Parent: -1, Ordinal: idx, PublisherTitle: g.Title, Status: "active"})
		for _, c := range g.Controls {
			if err := walk(c, idx, "control", groupParams); err != nil {
				return nil, err
			}
		}
	}
	return nodes, nil
}
