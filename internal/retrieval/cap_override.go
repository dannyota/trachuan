package retrieval

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// CapOverride sets a research-only document cap. Zero means no cap.
type CapOverride int

// ParseCapOverride parses a cap from 1 to 50 or none.
func ParseCapOverride(s string) (*CapOverride, error) {
	if s == "none" {
		v := CapOverride(0)
		return &v, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 50 || strconv.Itoa(n) != s {
		return nil, fmt.Errorf("invalid cap %q: expected 1 to 50 or none", s)
	}
	v := CapOverride(n)
	return &v, nil
}

// MarshalJSON encodes the cap as an integer or none.
func (c CapOverride) MarshalJSON() ([]byte, error) {
	if c == 0 {
		return json.Marshal("none")
	}
	if c < 1 || c > 50 {
		return nil, fmt.Errorf("invalid cap override")
	}
	return json.Marshal(int(c))
}

// UnmarshalJSON decodes an integer cap or none.
func (c *CapOverride) UnmarshalJSON(data []byte) error {
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		if n < 1 || n > 50 {
			return fmt.Errorf("invalid cap override")
		}
		*c = CapOverride(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil || s != "none" {
		return fmt.Errorf("invalid cap override")
	}
	*c = 0
	return nil
}

func effectiveCap(cap *CapOverride, original, limit int) (int, error) {
	if cap == nil {
		return original, nil
	}
	if *cap < 0 || *cap > 50 {
		return 0, fmt.Errorf("invalid cap override")
	}
	if *cap == 0 {
		return limit, nil
	}
	return int(*cap), nil
}

// ShapeWithCap shapes cached candidates using a research-only cap.
func ShapeWithCap(arms Arms, recipe Recipe, limit int, mode ArmMode, cap *CapOverride) (Response, error) {
	if limit == 0 {
		limit = 8
	}
	value, err := effectiveCap(cap, recipe.DocumentCap, limit)
	if err != nil {
		return Response{}, err
	}
	return shapeWithPolicy(arms, recipe, limit, mode, value, recipe.NonCurrentTail)
}
