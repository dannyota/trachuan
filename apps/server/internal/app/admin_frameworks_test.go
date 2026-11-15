package app

import (
	"context"
	"io"
	"testing"
)

func TestAdmin_FrameworksRejectsInvalidFlags(t *testing.T) {
	for _, args := range [][]string{
		{"frameworks"},
		{"frameworks", "unknown"},
		{"frameworks", "allow", "--email", "owner@example.test", "--scope", "*"},
		{"frameworks", "allow", "--email", "invalid", "--scope", "*", "--by", "operator"},
		{"frameworks", "allow", "--email", "owner@example.test", "--scope", "fw-*", "--by", "operator"},
		{"frameworks", "allow", "--email", "owner@example.test", "--scope", "*", "--by", " "},
		{"frameworks", "revoke", "--email", "owner@example.test", "--scope", "*", "--by", "operator", "--note", "not accepted"},
		{"frameworks", "list", "--email", "invalid"},
		{"frameworks", "list", "--email", ""},
		{"frameworks", "list", "--scope", "*"},
		{"frameworks", "list", "trailing"},
	} {
		if err := runAdmin(context.Background(), args, nil, io.Discard); err == nil || err.Error() != "invalid frameworks command" {
			t.Fatalf("args=%q error=%v", args, err)
		}
	}
}
func TestAdmin_RestoreFenceRejectsArguments(t *testing.T) {
	if err := runAdmin(context.Background(), []string{"restore-fence", "unexpected"}, nil, io.Discard); err == nil || err.Error() != "invalid restore-fence arguments" {
		t.Fatalf("error=%v", err)
	}
}
