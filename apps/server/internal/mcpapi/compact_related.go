package mcpapi

import (
	"encoding/json"

	"danny.vn/trachuan/internal/retrieval"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func compactRelatedResult(cfg Config, name string, in any, result *mcp.CallToolResult) (*mcp.CallToolResult, error) {
	if !cfg.OmitCompactRelated || name != "search" || in.(searchInput).Detail == retrieval.DetailFull {
		return result, nil
	}
	var response map[string]json.RawMessage
	if err := json.Unmarshal(result.StructuredContent.(json.RawMessage), &response); err != nil {
		return nil, retrieval.ErrUnavailable
	}
	if raw, ok := response["documents"]; ok {
		var documents map[string]map[string]json.RawMessage
		if err := json.Unmarshal(raw, &documents); err != nil {
			return nil, retrieval.ErrUnavailable
		}
		for _, doc := range documents {
			delete(doc, "related")
		}
		var err error
		response["documents"], err = json.Marshal(documents)
		if err != nil {
			return nil, retrieval.ErrUnavailable
		}
	}
	raw, err := json.Marshal(response)
	if err != nil {
		return nil, retrieval.ErrUnavailable
	}
	return serializedResult(raw), nil
}
