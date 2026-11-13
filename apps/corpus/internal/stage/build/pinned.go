package build

import (
	"encoding/json"
	"fmt"
	"time"
)

func (l *loader) pinnedMetadata(key, fetchHash string) (*time.Time, *string, error) {
	pin, ok := l.pins[key]
	if !ok {
		return nil, nil, nil
	}
	if pin.Fetch != fetchHash {
		return nil, nil, fmt.Errorf("pinned fetch hash mismatch for %s", key)
	}
	since, err := time.Parse("2006-01-02", pin.Since)
	if err != nil {
		return nil, nil, fmt.Errorf("pinned withdrawal date: %w", err)
	}
	data, err := json.Marshal(pin)
	if err != nil {
		return nil, nil, fmt.Errorf("encode retained source gap: %w", err)
	}
	found := false
	for _, g := range l.gaps {
		if g.ItemKey == key && g.Kind == "source_withdrawn_retained" {
			found = true
			break
		}
	}
	if !found {
		l.addGap("build", key, "source_withdrawn_retained", string(data))
	}
	return &since, &pin.Rel, nil
}
