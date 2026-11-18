package intake

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

func quote(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

func identity(i Identity) string {
	return "{ key: " + quote(i.Key) + ", anchor: " + quote(i.Anchor) + " }"
}

func Encode(records []Record) []byte {
	if len(records) == 0 {
		return []byte("[]\n")
	}
	var b strings.Builder
	for _, r := range records {
		b.WriteString("- origin: " + quote(r.Origin) + "\n  origin_id: " + quote(r.OriginID) + "\n  kind: " + quote(r.Kind) + "\n")
		if r.Target.Number != "" {
			b.WriteString("  target: { number: " + quote(r.Target.Number) + " }\n")
		} else {
			b.WriteString("  target: " + identity(Identity{r.Target.Key, r.Target.Anchor}) + "\n")
		}
		b.WriteString("  claim: { code: " + quote(r.Claim.Code))
		if r.Claim.Acting != (Identity{}) {
			b.WriteString(", acting: " + identity(r.Claim.Acting))
		}
		b.WriteString(" }\n")
	}
	return []byte(b.String())
}

func Merge(country string, existing File, add []Record) (merged, added []Record, remaining int, err error) {
	have := map[[2]string]Record{}
	merged = slices.Clone(existing.Records)
	for _, r := range merged {
		have[r.pair()] = r
	}
	var fresh []Record
	for _, r := range add {
		if problems := r.validate(country); len(problems) > 0 {
			return nil, nil, 0, &ValidationError{Problems: problems}
		}
		old, ok := have[r.pair()]
		if ok && old != r {
			return nil, nil, 0, fmt.Errorf("record %s/%s exists with other content", r.Origin, r.OriginID)
		}
		if !ok {
			have[r.pair()] = r
			fresh = append(fresh, r)
		}
	}
	slices.SortFunc(fresh, compare)
	if len(fresh) > MaxNewPerImport {
		remaining = len(fresh) - MaxNewPerImport
		fresh = fresh[:MaxNewPerImport]
	}
	added = fresh
	merged = append(merged, added...)
	if len(merged) > MaxRecords {
		return nil, nil, 0, errors.New("file would exceed the record limit")
	}
	slices.SortFunc(merged, compare)
	return merged, added, remaining, nil
}
