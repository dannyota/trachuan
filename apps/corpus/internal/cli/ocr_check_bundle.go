package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
)

func verifyCheckBundle(dir, key string) (string, error) {
	parts := strings.SplitN(key, ":", 3)
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid check document key")
	}
	names := []string{"files.json"}
	switch parts[1] {
	case "vbpl":
		names = append(names, "detail.json", "tree.json")
	case "chinhphu":
		names = append(names, "document.json", "metadata.json", "detail.html")
	default:
		return "", fmt.Errorf("unknown check document source")
	}
	hash, err := artifact.VerifyFiles(dir, names...)
	if err != nil {
		return "", err
	}
	doc, err := fetch.ReadDocument(dir, key)
	if err != nil {
		return "", err
	}
	for _, f := range doc.Files {
		if f.Name == parts[2]+"_content.html" {
			if _, err = artifact.VerifyFiles(dir, filepath.Join("files", f.SHA256)); err != nil {
				return "", err
			}
		}
	}
	// PDFs are verified separately under the size bound so corruption or absence
	// becomes a recorded failed attempt rather than aborting the whole command.
	return hash, nil
}
