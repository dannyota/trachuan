//go:build faultinject

// Package fault provides fault injection hooks.
package fault

import (
	"fmt"
	"os"
)

// Hit exits at the configured point and zero-based batch index.
func Hit(point string, n int) {
	if os.Getenv("CORPUS_FAULT") == fmt.Sprintf("%s:%d", point, n) {
		os.Exit(3)
	}
}
