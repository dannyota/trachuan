package discover

import (
	"sort"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/pinned"
	"danny.vn/trachuan/apps/corpus/internal/source"
)

func livePins(ids []string, pins []source.Pin, complete bool) []pinned.Notice {
	notices := []pinned.Notice{}
	if !complete {
		return notices
	}
	listed := map[string]bool{}
	for _, id := range ids {
		listed[id] = true
	}
	for _, pin := range pins {
		_, id, _ := strings.Cut(pin.Key, ":vbpl:")
		if listed[id] {
			notices = append(notices, pinned.Notice{Kind: "source_pin_live", Key: pin.Key})
		}
	}
	sort.Slice(notices, func(i, j int) bool { return notices[i].Key < notices[j].Key })
	return notices
}
