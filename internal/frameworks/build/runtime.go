package build

import (
	"runtime"
	"runtime/debug"
)

func lexicalRuntime() map[string]string {
	versions := map[string]string{"go": runtime.Version(), "golang.org/x/text": ""}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return versions
	}
	for _, dep := range info.Deps {
		if dep.Path != "golang.org/x/text" {
			continue
		}
		if dep.Replace != nil {
			dep = dep.Replace
		}
		versions["golang.org/x/text"] = dep.Version
	}
	return versions
}
