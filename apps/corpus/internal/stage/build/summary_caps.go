package build

import (
	"encoding/json"
	"fmt"
)

type summaryCapStep string

const (
	summaryWhole           summaryCapStep = "whole"
	summaryHeadingsDropped summaryCapStep = "headings_dropped"
	summaryOutlineDropped  summaryCapStep = "outline_dropped"
)

type SummaryStats struct {
	HeadingsDropped int64 `json:"headings_dropped"`
	OutlineDropped  int64 `json:"outline_dropped"`
}

func capSummary(d summaryDocument) (summaryDocument, summaryCapStep, error) {
	for step := summaryWhole; ; {
		b, err := json.Marshal(d)
		if err != nil {
			return summaryDocument{}, step, fmt.Errorf("encode summary: %w", err)
		}
		if len(d.Payload.Outline) <= 1500 && len(b) <= 65536 {
			return d, step, nil
		}
		switch step {
		case summaryWhole:
			for i := range d.Payload.Outline {
				d.Payload.Outline[i].Heading = ""
			}
			step = summaryHeadingsDropped
		case summaryHeadingsDropped:
			d.Payload.Outline = []summaryOutline{}
			d.Indexed = false
			step = summaryOutlineDropped
		case summaryOutlineDropped:
			return summaryDocument{}, step, fmt.Errorf("summary %s: serialized row exceeds 65536 octets", d.Key)
		}
	}
}
