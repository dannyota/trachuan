package weblogin

import (
	"strings"
	"testing"
)

func TestNext_RejectsOffsitePaths(t *testing.T) {
	rejected := []string{"", "https://attacker.example", "//attacker.example", `/\attacker.example`, `/a\b`, "relative", "///a", "/%2fattacker.example", "/%5cattacker.example", "/a%5cb", "/a%00b", "/a%1fb", "/a%7fb", "/a\nb", "/a\rb", "/a\tb", "/a\x00b", "/a\x7fb", "/%", "/%zz", "/.", "/..", "/./vn", "/vn/../search", "/%2e/vn", "/vn/%2e%2e/search", "/a%2f..%2fb", "/.//attacker.example", "/?x=%zz", "/?x=%5c", "/?x=%0a", "/#%00", "/" + strings.Repeat("a", 512)}
	for _, raw := range rejected {
		t.Run(raw, func(t *testing.T) {
			if got := ValidateNext(raw); got != "/" {
				t.Errorf("ValidateNext(%q)=%q", raw, got)
			}
		})
	}
	for _, raw := range []string{"/", "/vn/search?q=law&x=1", "/vn/documents/a%20b#section", "/a/.../b", "/a..b", "/?url=https://example.test", "/" + strings.Repeat("a", 511), "/%252fexample"} {
		t.Run("valid "+raw, func(t *testing.T) {
			if got := ValidateNext(raw); got != raw {
				t.Errorf("ValidateNext(%q)=%q", raw, got)
			}
		})
	}
}

func TestStart_ValidatesNextBeforeStoring(t *testing.T) {
	f := newFixture(t)
	for _, raw := range []string{"//attacker.example", "/%2fattacker.example", "/vn/../search", "/?x=%zz"} {
		a := f.begin(t, raw)
		if a.tx.NextPath != "/" {
			t.Errorf("stored hostile next %q", a.tx.NextPath)
		}
	}
}
