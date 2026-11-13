package extract

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestExtract_LegacyWholeTreeProcessBytesUnchanged(t *testing.T) {
	ctx := context.Background()
	bundle := headingBundle(t, bareTree, true)
	artifacts := artifact.Store{Root: t.TempDir()}
	fetched, err := artifacts.PublishContent(ctx, "fetch", strings.Repeat("a", 64), func(dir string) error { return os.CopyFS(dir, os.DirFS(bundle)) })
	if err != nil {
		t.Fatal(err)
	}
	db := processFixtureDB(t, fetched)
	output, err := New().Process(ctx, stage.Env{DB: db, Artifacts: artifacts}, stage.Run{ID: 1}, stage.Item{Key: "vn:vbpl:1", InputHash: InputHash(fetched.SHA256, "html_transcription")})
	if err != nil || output.Artifact == nil {
		t.Fatalf("output=%+v err=%v", output, err)
	}
	text, err := os.ReadFile(filepath.Join(artifacts.Abs(output.Artifact.Path), "text.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Pinned through Process with dc00aff selection, extraction and text-record code.
	textHash := fmt.Sprintf("%x", sha256.Sum256(text))
	if textHash != "f949732dda53a4f2f4558dd9847e5b8438b9a3d4612815c38ba4db4df54cd7b6" || output.Artifact.SHA256 != "3e7d39ecc68a43335c2b797fb2dbb6f7466dad6d2fc8c4d0dfd7b8db63d5295e" {
		t.Fatalf("text.json=%s extract manifest=%s", textHash, output.Artifact.SHA256)
	}
}
