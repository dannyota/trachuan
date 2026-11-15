package app

import (
	"errors"
	"os"
	"strings"
)

func loadMCPOriginConfig() ([]string, string, error) {
	mode, present := os.LookupEnv("SERVER_MCP_ORIGIN_MODE")
	if !present {
		mode = "enforce"
	}
	if mode != "log" && mode != "enforce" {
		return nil, "", errors.New("invalid SERVER_MCP_ORIGIN_MODE")
	}
	var origins []string
	if raw := strings.TrimSpace(os.Getenv("SERVER_MCP_ORIGINS")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			origin := strings.TrimSpace(part)
			if !validMCPOrigin(origin) {
				return nil, "", errors.New("invalid SERVER_MCP_ORIGINS")
			}
			origins = append(origins, origin)
		}
	}
	return origins, mode, nil
}
