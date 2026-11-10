package tokenizer

import (
	"runtime/debug"
	"strings"
)

// Version returns the linked tokenizers module version without its v prefix.
func Version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, dependency := range info.Deps {
		if dependency.Path == "github.com/daulet/tokenizers" {
			return strings.TrimPrefix(dependency.Version, "v")
		}
	}
	return ""
}
