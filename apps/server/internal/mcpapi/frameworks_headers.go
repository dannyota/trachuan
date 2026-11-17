package mcpapi

import (
	"encoding/json"
	"strings"
)

func frameworkToolRequest(params json.RawMessage) bool {
	var in struct {
		Name      json.RawMessage `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal(params, &in) != nil {
		return false
	}
	var name, corpus string
	_ = json.Unmarshal(in.Name, &name)
	if name == "mappings" {
		return true
	}
	var arguments map[string]json.RawMessage
	if json.Unmarshal(in.Arguments, &arguments) != nil {
		return false
	}
	_ = json.Unmarshal(arguments["corpus"], &corpus)
	if corpus == "frameworks" {
		return true
	}
	if name != "read" {
		return false
	}
	var items []struct {
		Handle string `json:"handle"`
	}
	_ = json.Unmarshal(arguments["items"], &items)
	for _, item := range items {
		if strings.HasPrefix(item.Handle, "fw:") {
			return true
		}
	}
	return false
}
