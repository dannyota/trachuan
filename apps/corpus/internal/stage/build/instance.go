package build

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type ownershipMarker struct {
	RunKey     string `json:"run_key"`
	InputHash  string `json:"build_input_hash"`
	RecipeHash string `json:"build_recipe_hash"`
	Instance   string `json:"instance"`
}

func encodeOwnershipMarker(marker ownershipMarker) (string, error) {
	if err := validateOwnershipMarker(marker); err != nil {
		return "", err
	}
	data, err := json.Marshal(marker)
	if err != nil {
		return "", fmt.Errorf("encode ownership marker: %w", err)
	}
	return string(data), nil
}

func validateOwnershipMarker(marker ownershipMarker) error {
	parts := strings.Split(marker.RunKey, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid marker run key")
	}
	if _, err := DatabaseName(parts[0], parts[1]); err != nil {
		return fmt.Errorf("invalid marker run key: %w", err)
	}
	for _, hash := range []string{marker.InputHash, marker.RecipeHash} {
		if len(hash) != 64 || strings.ToLower(hash) != hash {
			return fmt.Errorf("invalid marker hash")
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return fmt.Errorf("invalid marker hash: %w", err)
		}
	}
	if marker.Instance != "built" && marker.Instance != "restored" {
		return fmt.Errorf("invalid marker instance")
	}
	return nil
}

func parseOwnershipMarker(raw string) (ownershipMarker, error) {
	var marker ownershipMarker
	if strings.HasPrefix(raw, "{") {
		if err := json.Unmarshal([]byte(raw), &marker); err != nil {
			return marker, fmt.Errorf("decode ownership marker: %w", err)
		}
	} else {
		parts := strings.Split(raw, ":")
		if len(parts) != 3 {
			return marker, fmt.Errorf("invalid ownership marker")
		}
		marker = ownershipMarker{RunKey: parts[0], InputHash: parts[1], RecipeHash: parts[2], Instance: "built"}
	}
	if err := validateOwnershipMarker(marker); err != nil {
		return ownershipMarker{}, err
	}
	return marker, nil
}

func sameOwnership(first, second string) bool {
	a, err := parseOwnershipMarker(first)
	if err != nil {
		return false
	}
	b, err := parseOwnershipMarker(second)
	if err != nil {
		return false
	}
	return a.RunKey == b.RunKey && a.InputHash == b.InputHash && a.RecipeHash == b.RecipeHash
}

func markRestored(ctx context.Context, admin *pgx.Conn, name string) error {
	raw, err := readMarker(ctx, admin, name)
	if err != nil {
		return err
	}
	marker, err := parseOwnershipMarker(raw)
	if err != nil {
		return fmt.Errorf("read restored database ownership: %w", err)
	}
	marker.Instance = "restored"
	raw, err = encodeOwnershipMarker(marker)
	if err != nil {
		return err
	}
	if err = writeMarker(ctx, admin, name, raw); err != nil {
		return fmt.Errorf("mark restored database: %w", err)
	}
	return nil
}

func databaseInstance(ctx context.Context, admin *pgx.Conn, name string) (string, error) {
	raw, err := readMarker(ctx, admin, name)
	if err != nil {
		return "", err
	}
	marker, err := parseOwnershipMarker(raw)
	if err != nil {
		return "", fmt.Errorf("read database instance: %w", err)
	}
	return marker.Instance, nil
}
