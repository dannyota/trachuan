//go:build ort

package tokenizer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func openTestTokenizer(t *testing.T) *Tokenizer {
	t.Helper()
	tok, err := Open("../../testdata/tokenizer.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tok.Close(); err != nil {
			t.Error(err)
		}
	})
	return tok
}

func TestTokenizer_SpecialTokensAndNoTruncation(t *testing.T) {
	tok := openTestTokenizer(t)
	ids, err := tok.Encode("hello world")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids, []int64{1, 3, 4, 2}) {
		t.Fatalf("ids = %v", ids)
	}
	ids, err = tok.Encode(strings.Repeat("hello ", 1024))
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1026 || ids[0] != 1 || ids[len(ids)-1] != 2 {
		t.Fatalf("count = %d", len(ids))
	}
}

func TestTokenizer_CloseAndInvalidFile(t *testing.T) {
	tok := openTestTokenizer(t)
	if err := tok.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := tok.Encode("hello"); err == nil {
		t.Fatal("Encode after Close succeeded")
	}
	if _, err := Open(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("Open missing file succeeded")
	}
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("Open invalid file succeeded")
	}
}

func TestVersion_ModuleVersion(t *testing.T) {
	if got := Version(); got != "1.27.0" {
		t.Fatalf("Version = %q", got)
	}
}

func TestTokenizer_RejectsInputThatCStringCannotRepresent(t *testing.T) {
	tok := openTestTokenizer(t)
	for _, text := range []string{"hello\x00world", "hello\xffworld"} {
		if _, err := tok.Encode(text); err == nil {
			t.Fatal("unrepresentable input accepted")
		}
	}
}

func TestTokenizer_RefusesSavedTruncationOrPadding(t *testing.T) {
	data, err := os.ReadFile("../../testdata/tokenizer.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range []struct {
		name, value string
	}{
		{"truncation", `{"direction":"Right","max_length":4,"strategy":"LongestFirst","stride":0}`},
		{"padding", `{"strategy":{"Fixed":16},"direction":"Right","pad_to_multiple_of":null,"pad_id":0,"pad_type_id":0,"pad_token":"<unk>"}`},
	} {
		t.Run(setting.name, func(t *testing.T) {
			var document map[string]json.RawMessage
			if err := json.Unmarshal(data, &document); err != nil {
				t.Fatal(err)
			}
			document[setting.name] = json.RawMessage(setting.value)
			encoded, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "tokenizer.json")
			if err = os.WriteFile(path, encoded, 0600); err != nil {
				t.Fatal(err)
			}
			tok, err := Open(path)
			if err == nil {
				_ = tok.Close()
				t.Fatal("enabled saved settings accepted")
			}
		})
	}
}
