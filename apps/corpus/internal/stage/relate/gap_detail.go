package relate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"unicode/utf8"
)

// The workshop gap table indexes detail (a btree entry holds at most 2704
// bytes), so every relate gap detail is bounded here. Full evidence stays in
// the relate artifact.
const (
	gapDetailBytes   = 512
	gapNodeBytes     = 128
	runGapDetailText = 200 // runes of sentence text copied into a text gap
)

// boundedDetail returns s unchanged when it fits; otherwise a valid UTF-8
// prefix followed by a hash of the whole value, so distinct long values stay
// distinct.
func boundedDetail(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	sum := sha256.Sum256([]byte(s))
	suffix := "#" + hex.EncodeToString(sum[:6])
	cut := limit - len(suffix)
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + suffix
}

// runGapDetail is the bounded run-gap form of a text gap: its position in the
// artifact report (unique per item), the source node and the start of the text.
func runGapDetail(index int, gap textGap) string {
	text := []rune(gap.Detail)
	if len(text) > runGapDetailText {
		text = text[:runGapDetailText]
	}
	return fmt.Sprintf("%d:%s:%s", index, boundedDetail(gap.SourceNodeID, gapNodeBytes), string(text))
}
