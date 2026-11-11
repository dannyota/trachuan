package ocrcheck

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type runtimeAccess struct{ files, dirs []string }

func resolveRuntime(binaries []string) (runtimeAccess, error) {
	files, err := resolveLibraries(binaries)
	if err != nil {
		return runtimeAccess{}, err
	}
	access := runtimeAccess{files: files}
	for _, path := range files {
		if !slices.Contains(binaries, path) && !systemLibrary(path) && !strings.HasPrefix(path, "/opt/homebrew/") && !strings.HasPrefix(path, "/usr/local/") {
			return runtimeAccess{}, rendererConfiguration()
		}
	}
	for _, dir := range []string{"/System/Library/dyld", "/System/Volumes/Preboot/Cryptexes/OS/System/Library/dyld", "/private/var/db/dyld"} {
		entries, readErr := os.ReadDir(dir)
		if os.IsNotExist(readErr) {
			continue
		}
		if readErr != nil {
			return runtimeAccess{}, rendererConfiguration()
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), "dyld_shared_cache_") && !strings.HasSuffix(entry.Name(), ".map") {
				access.files = append(access.files, filepath.Join(dir, entry.Name()))
			}
		}
	}
	for _, path := range []string{"/usr/lib/dyld", "/private/etc/localtime", "/usr/share/locale/locale.alias"} {
		real, resolveErr := filepath.EvalSymlinks(path)
		if os.IsNotExist(resolveErr) {
			continue
		}
		if resolveErr != nil {
			return runtimeAccess{}, rendererConfiguration()
		}
		access.files = append(access.files, path, real)
	}
	// Fontconfig may follow symlinks from its configuration into share/fontconfig.
	// Global font/data roots suffice; no user font or home directory is readable.
	for _, dir := range []string{
		"/System/Library/Fonts", "/Library/Fonts", "/private/etc/fonts", "/usr/share/fonts", "/usr/share/locale/C",
		"/opt/homebrew/etc/fonts", "/opt/homebrew/share/fontconfig", "/opt/homebrew/share/fonts", "/opt/homebrew/share/poppler", "/opt/homebrew/var/cache/fontconfig",
		"/usr/local/etc/fonts", "/usr/local/share/fontconfig", "/usr/local/share/fonts", "/usr/local/share/poppler", "/usr/local/var/cache/fontconfig",
	} {
		real, resolveErr := filepath.EvalSymlinks(dir)
		if os.IsNotExist(resolveErr) {
			continue
		}
		if resolveErr != nil {
			return runtimeAccess{}, rendererConfiguration()
		}
		info, statErr := os.Stat(real)
		if statErr != nil || !info.IsDir() || !fontRuntimeDirectory(real) {
			return runtimeAccess{}, rendererConfiguration()
		}
		access.dirs = append(access.dirs, dir, real)
	}
	slices.Sort(access.files)
	access.files = slices.Compact(access.files)
	slices.Sort(access.dirs)
	access.dirs = slices.Compact(access.dirs)
	return access, nil
}

func fontRuntimeDirectory(path string) bool {
	for _, system := range []string{"/System/Library/Fonts", "/Library/Fonts", "/private/etc/fonts", "/usr/share/fonts", "/usr/share/locale/C"} {
		if path == system {
			return true
		}
	}
	if !strings.HasPrefix(path, "/opt/homebrew/") && !strings.HasPrefix(path, "/usr/local/") {
		return false
	}
	for _, suffix := range []string{"/etc/fonts", "/share/fontconfig", "/share/fonts", "/share/poppler", "/var/cache/fontconfig"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}
