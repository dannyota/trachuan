package retrieval

import (
	"strings"
	"testing"
	"unicode"
)

func TestSnippet_ShortHitIsWholeText(t *testing.T) {
	text := strings.Repeat("a", 300)
	got, complete := Snippet(Hit{Text: text, Complete: true}, 600)
	if got != text || !complete {
		t.Fatalf("got %q, complete %v", got, complete)
	}
}

func TestSnippet_ShortButRecipeIncomplete(t *testing.T) {
	got, complete := Snippet(Hit{Text: "short"}, 600)
	if got != "short" || complete {
		t.Fatalf("got %q, complete %v", got, complete)
	}
}

func TestSnippet_LeadThenBody(t *testing.T) {
	lead := strings.Repeat("l", 100)
	h := Hit{Text: lead + "\n" + strings.Repeat("body ", 200), Complete: true, BestTextStart: 101, Fragments: []Fragment{{Role: "lead", TextEnd: 100}}}
	got, complete := Snippet(h, 600)
	if !strings.HasPrefix(got, lead+"\nbody ") || !strings.HasSuffix(got, " […]") || len([]rune(got)) > 600 || complete {
		t.Fatalf("got %q, complete %v", got, complete)
	}
}

func TestSnippet_LongLeadCutAtHalf(t *testing.T) {
	h := Hit{Text: strings.Repeat("l", 900) + "\n" + strings.Repeat("b", 900), BestTextStart: 901, Fragments: []Fragment{{Role: "lead", TextEnd: 900}}}
	got, _ := Snippet(h, 600)
	i := strings.Index(got, " […]")
	if i < 0 || len([]rune(got[:i+len(" […]")])) > 300 {
		t.Fatalf("lead not cut within half: %q", got)
	}
}

func TestSnippet_BodyNotAtStartGetsLeadingMarker(t *testing.T) {
	h := Hit{Text: "lead\n" + strings.Repeat("x", 50) + strings.Repeat("body ", 150), BestTextStart: 55, Fragments: []Fragment{{Role: "lead", TextEnd: 4}}}
	got, _ := Snippet(h, 600)
	if !strings.HasPrefix(got, "lead […] body ") {
		t.Fatalf("missing marker: %q", got)
	}
}

func TestSnippet_CutAtWhitespace(t *testing.T) {
	got, _ := Snippet(Hit{Text: strings.Repeat("word ", 200)}, 600)
	if !strings.HasSuffix(got, "word […]") {
		t.Fatalf("cut inside word: %q", got)
	}
}

func TestSnippet_NoWhitespaceKeepsCombiningMark(t *testing.T) {
	got, _ := Snippet(Hit{Text: strings.Repeat("a\u0301", 350)}, 600)
	prefix := strings.TrimSuffix(got, " […]")
	r := []rune(prefix)
	if len(r) == 0 || !unicode.Is(unicode.Mn, r[len(r)-1]) {
		t.Fatalf("dropped combining mark: %q", got)
	}
}

func TestSnippet_MarkersCountTowardCap(t *testing.T) {
	h := Hit{Text: strings.Repeat("l", 900) + "\n" + strings.Repeat("b", 900), BestTextStart: 950, Fragments: []Fragment{{Role: "lead", TextEnd: 900}}}
	got, complete := Snippet(h, 600)
	if len([]rune(got)) > 600 || complete || strings.Count(got, "[…]") != 2 {
		t.Fatalf("got %d runes, complete %v: %q", len([]rune(got)), complete, got)
	}
}

func TestCutAt_WhitespaceAtLimit(t *testing.T) {
	r := []rune("one two three")
	if got := cutAt(r, 7); got != 7 {
		t.Fatalf("cutAt = %d, want 7", got)
	}
}

func TestSnippet_SmallCaps(t *testing.T) {
	h := Hit{Text: "L\nabcdefghij", BestTextStart: 3, Fragments: []Fragment{{Role: "lead", TextEnd: 1}}}
	for capRunes := 1; capRunes <= 6; capRunes++ {
		got, complete := Snippet(h, capRunes)
		if len([]rune(got)) > capRunes || complete {
			t.Errorf("cap %d: got %q, complete %v", capRunes, got, complete)
		}
	}
}

func TestSnippet_LongCombiningSequence(t *testing.T) {
	source := "a" + strings.Repeat("\u0301", 699)
	got, complete := Snippet(Hit{Text: source}, 600)
	want := string([]rune(source)[:596]) + " " + OmissionMarker
	if got != want || len([]rune(got)) > 600 || complete {
		t.Fatalf("unexpected cut: length %d, complete %v", len([]rune(got)), complete)
	}
}

func TestCutAt_CombiningSequence(t *testing.T) {
	r := []rune(strings.Repeat("a\u0301", 5))
	if got := cutAt(r, 5); got != 4 {
		t.Fatalf("cutAt = %d, want 4", got)
	}
}

func TestCutAt_OneRuneNonpositiveLimit(t *testing.T) {
	for _, limit := range []int{0, -1} {
		if got := cutAt([]rune("x"), limit); got != 1 {
			t.Errorf("limit %d: got %d", limit, got)
		}
	}
}
