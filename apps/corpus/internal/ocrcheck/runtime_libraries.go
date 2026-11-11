package ocrcheck

import (
	"debug/macho"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type machOLinks struct{ libraries, rpaths []string }

func readMachOLinks(path string) (machOLinks, error) {
	var result machOLinks
	add := func(file *macho.File) error {
		libs, err := file.ImportedLibraries()
		if err != nil {
			return rendererConfiguration()
		}
		result.libraries = append(result.libraries, libs...)
		for _, load := range file.Loads {
			if rpath, ok := load.(*macho.Rpath); ok {
				result.rpaths = append(result.rpaths, rpath.Path)
				continue
			}
			raw := load.Raw()
			if len(raw) < 8 {
				return rendererConfiguration()
			}
			// debug/macho exposes weak, re-exported and upward imports as raw commands.
			switch file.ByteOrder.Uint32(raw) {
			case 0x80000018, 0x8000001f, 0x80000023, 0x20:
				if len(raw) < 24 {
					return rendererConfiguration()
				}
				offset := file.ByteOrder.Uint32(raw[8:])
				if offset >= uint32(len(raw)) {
					return rendererConfiguration()
				}
				name := string(raw[offset:])
				end := strings.IndexByte(name, 0)
				if end < 1 {
					return rendererConfiguration()
				}
				result.libraries = append(result.libraries, name[:end])
			}
		}
		return nil
	}
	fat, err := macho.OpenFat(path)
	if err == nil {
		defer func() { _ = fat.Close() }()
		for _, arch := range fat.Arches {
			if err = add(arch.File); err != nil {
				return machOLinks{}, err
			}
		}
	} else {
		if !errors.Is(err, macho.ErrNotFat) {
			return result, rendererConfiguration()
		}
		file, openErr := macho.Open(path)
		if openErr != nil {
			return result, rendererConfiguration()
		}
		defer func() { _ = file.Close() }()
		if err = add(file); err != nil {
			return machOLinks{}, err
		}
	}
	return result, nil
}

func resolveLibraries(roots []string) ([]string, error) {
	if len(roots) == 0 {
		return nil, rendererConfiguration()
	}
	files := map[string]bool{}
	visited := map[string]bool{}
	parsed := map[string]machOLinks{}
	var walk func(string, string, []string) error
	walk = func(path, executable string, inherited []string) error {
		if !filepath.IsAbs(path) {
			return rendererConfiguration()
		}
		path = filepath.Clean(path)
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			// Modern macOS supplies these images from its read-only dyld shared cache.
			// Both tool startup probes must succeed before any document can be rendered.
			if os.IsNotExist(err) && systemLibrary(path) {
				files[path] = true
				return nil
			}
			return rendererConfiguration()
		}
		files[path] = true
		files[real] = true
		links, ok := parsed[real]
		if !ok {
			links, err = readMachOLinks(real)
			if err != nil {
				return err
			}
			parsed[real] = links
		}
		search := make([]string, 0, len(links.rpaths)+len(inherited))
		for _, rpath := range links.rpaths {
			expanded, expandErr := expandLoaderPath(rpath, real, executable)
			if expandErr != nil {
				return expandErr
			}
			if !slices.Contains(search, expanded) {
				search = append(search, expanded)
			}
		}
		for _, rpath := range inherited {
			if !slices.Contains(search, rpath) {
				search = append(search, rpath)
			}
		}
		key := real + "\x00" + executable + "\x00" + strings.Join(search, "\x00")
		if visited[key] {
			return nil
		}
		if len(visited) >= 4096 {
			return rendererConfiguration()
		}
		visited[key] = true
		for _, library := range links.libraries {
			var dependency string
			if strings.HasPrefix(library, "@rpath/") {
				for _, rpath := range search {
					candidate := filepath.Join(rpath, strings.TrimPrefix(library, "@rpath/"))
					if _, statErr := os.Stat(candidate); statErr == nil {
						dependency = candidate
						break
					}
				}
				if dependency == "" {
					return rendererConfiguration()
				}
			} else {
				dependency, err = expandLoaderPath(library, real, executable)
				if err != nil {
					return err
				}
			}
			if err = walk(dependency, executable, search); err != nil {
				return err
			}
		}
		return nil
	}
	for _, root := range roots {
		real, err := filepath.EvalSymlinks(root)
		if err != nil {
			return nil, rendererConfiguration()
		}
		if err = walk(root, real, nil); err != nil {
			return nil, err
		}
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	return paths, nil
}

func expandLoaderPath(path, loader, executable string) (string, error) {
	switch {
	case path == "@loader_path":
		path = filepath.Dir(loader)
	case path == "@executable_path":
		path = filepath.Dir(executable)
	case strings.HasPrefix(path, "@loader_path/"):
		path = filepath.Join(filepath.Dir(loader), strings.TrimPrefix(path, "@loader_path/"))
	case strings.HasPrefix(path, "@executable_path/"):
		path = filepath.Join(filepath.Dir(executable), strings.TrimPrefix(path, "@executable_path/"))
	case !filepath.IsAbs(path):
		return "", rendererConfiguration()
	}
	return filepath.Clean(path), nil
}

func systemLibrary(path string) bool {
	return strings.HasPrefix(path, "/usr/lib/") || strings.HasPrefix(path, "/System/Library/Frameworks/") || strings.HasPrefix(path, "/System/Library/PrivateFrameworks/")
}
