package fetch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestRows_RejectsMixedFormats(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"detail.json":   `{"data":{"id":"1"}}`,
		"document.json": `{"version":2,"document":{}}`,
		"files.json":    `[]`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	_, err := New(nil).Rows(context.Background(), stage.Env{Artifacts: artifact.Store{Root: dir}}, artifact.Result{Path: "."})
	if err == nil {
		t.Fatal("mixed formats accepted")
	}
}

func TestResolve_RejectsPlannedKeyMismatch(t *testing.T) {
	s, env := setup(t, &fixtureServer{})
	out := process(t, s, env)
	_, err := s.Resolve(context.Background(), env, stage.Run{}, stage.Item{Key: "vn:vbpl:other"}, *out.Artifact)
	if err == nil {
		t.Fatal("accepted artifact for a different planned key")
	}
}

func TestRows_DualFormatReader(t *testing.T) {
	vbpl, vbplEnv := setup(t, &fixtureServer{})
	vbplOutput := process(t, vbpl, vbplEnv)
	if _, err := vbpl.Rows(context.Background(), vbplEnv, *vbplOutput.Artifact); err != nil {
		t.Fatalf("VBPL rows: %v", err)
	}
	token := "abc"
	s, env := secondaryFixture(t, false, &token)
	out := secondaryFetch(t, s, env)
	if _, err := s.Rows(context.Background(), env, *out.Artifact); err != nil {
		t.Fatalf("secondary rows: %v", err)
	}
	_, err := s.Resolve(context.Background(), env, stage.Run{}, stage.Item{Key: "vn:chinhphu:1"}, *out.Artifact)
	if err == nil {
		t.Fatal("secondary identity mismatch accepted")
	}
}

func TestRows_RejectsInvalidSecondaryBundles(t *testing.T) {
	for _, tc := range []struct {
		name   string
		file   string
		change func(map[string]any)
	}{
		{"document version", "document.json", func(v map[string]any) { v["version"] = 3 }},
		{"file version", "files.json", func(v map[string]any) { v["version"] = 3 }},
		{"null file list", "files.json", func(v map[string]any) { v["files"] = nil }},
		{"metadata version", "metadata.json", func(v map[string]any) { v["version"] = 3 }},
		{"document identity", "document.json", func(v map[string]any) { v["document"].(map[string]any)["key"] = "vn:chinhphu:9" }},
		{"file source", "files.json", func(v map[string]any) { v["files"].([]any)[0].(map[string]any)["source"] = "vbpl" }},
		{"file role", "files.json", func(v map[string]any) { v["files"].([]any)[0].(map[string]any)["role"] = "invented" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token := "abc"
			s, env := secondaryFixture(t, false, &token)
			out := secondaryFetch(t, s, env)
			p := filepath.Join(env.Artifacts.Abs(out.Artifact.Path), tc.file)
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			var data map[string]any
			if err = json.Unmarshal(b, &data); err != nil {
				t.Fatal(err)
			}
			tc.change(data)
			b, err = json.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(p, b, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Rows(context.Background(), env, *out.Artifact); err == nil {
				t.Fatal("invalid bundle accepted")
			}
		})
	}
}

func TestRows_RejectsChangedIdentityEvidence(t *testing.T) {
	token := "abc"
	s, env := secondaryFixture(t, false, &token)
	out := secondaryFetch(t, s, env)
	path := filepath.Join(env.Artifacts.Abs(out.Artifact.Path), "detail.html")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("changed")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Rows(context.Background(), env, *out.Artifact); err == nil {
		t.Fatal("changed identity evidence accepted")
	}
}
