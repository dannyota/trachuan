package build

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"danny.vn/trachuan/internal/frameworks/registry"
)

func readRegistry(path string) (*registry.Registry, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	reg, err := registry.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, hide("validate registry", err)
	}
	return reg, nil
}

func releaseVersions(reg *registry.Registry) []registry.Framework {
	var versions []registry.Framework
	for _, v := range reg.Frameworks {
		if v.Parser != "" {
			versions = append(versions, v)
		}
	}
	sort.Slice(versions, func(i, j int) bool {
		a, b := versions[i], versions[j]
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		ad, bd := "", ""
		if a.Released != nil {
			ad = *a.Released
		}
		if b.Released != nil {
			bd = *b.Released
		}
		if ad != bd {
			return ad < bd
		}
		if a.Ordinal != b.Ordinal {
			return a.Ordinal < b.Ordinal
		}
		return a.Version < b.Version
	})
	return versions
}

func snapshot(ctx context.Context, dir string) (map[string]*string, error) {
	data, err := readFile("sources/frameworks.yaml")
	if err != nil {
		return nil, err
	}
	reg, err := registry.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, hide("validate registry", err)
	}
	if len(releaseVersions(reg)) == 0 {
		return nil, fmt.Errorf("no release versions")
	}
	if err = writeFile(dir, "sources/frameworks.yaml", data); err != nil {
		return nil, err
	}
	for _, v := range releaseVersions(reg) {
		for _, file := range v.Files {
			if contextErr := ctx.Err(); contextErr != nil {
				return nil, contextErr
			}
			source, readErr := reg.Read(file.Path)
			if readErr != nil {
				return nil, hide("read registered source", readErr)
			}
			if err = writeFile(dir, "files/"+file.Path, source); err != nil {
				return nil, err
			}
		}
	}
	root, err := os.OpenRoot(os.Getenv("FRAMEWORKS_SOURCE_DIR"))
	if err != nil {
		return nil, hide("open private inputs", err)
	}
	defer func() { _ = root.Close() }()
	for _, name := range []string{"trachuan/titles.csv", "trachuan/ocr-waivers.yaml"} {
		info, err := root.Stat(name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, hide("inspect private input", err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("private input is not a regular file")
		}
		data, err := root.ReadFile(name)
		if err != nil {
			return nil, hide("read private input", err)
		}
		if err := writeFile(dir, name, data); err != nil {
			return nil, err
		}
	}
	if err := writeRows(dir, "gaps.jsonl", []Gap{}); err != nil {
		return nil, err
	}
	return snapshotInputs(dir)
}

func snapshotInputs(dir string) (map[string]*string, error) {
	if _, err := outputHashes(dir); err != nil {
		return nil, err
	}
	reg, err := readRegistry(filepath.Join(dir, "sources/frameworks.yaml"))
	if err != nil {
		return nil, err
	}
	names := []string{"sources/frameworks.yaml", "trachuan/titles.csv", "trachuan/ocr-waivers.yaml"}
	for _, v := range releaseVersions(reg) {
		for _, file := range v.Files {
			names = append(names, "files/"+file.Path)
		}
	}
	inputs := map[string]*string{}
	for _, name := range names {
		data, err := readFile(filepath.Join(dir, name))
		if errors.Is(err, fs.ErrNotExist) && (name == "trachuan/titles.csv" || name == "trachuan/ocr-waivers.yaml") {
			inputs[name] = nil
			continue
		}
		if err != nil {
			return nil, err
		}
		hash := digest(data)
		inputs[name] = &hash
	}
	return inputs, nil
}
