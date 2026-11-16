package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
)

type recipe struct {
	Name       string         `json:"name"`
	Version    int            `json:"version"`
	Parameters map[string]any `json:"parameters"`
}

type manifest struct {
	Recipe  recipe             `json:"recipe"`
	Inputs  map[string]*string `json:"inputs"`
	Outputs map[string]string  `json:"outputs"`
}

func digest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

func readFile(path string) ([]byte, error) {
	dir, err := checkedDir(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	path = filepath.Join(dir, filepath.Base(path))
	info, err := os.Lstat(path)
	if err != nil {
		return nil, hide("inspect artifact", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("artifact is not a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, hide("read artifact", err)
	}
	return data, nil
}

func writeFile(dir, name string, data []byte) error {
	dir, err := checkedDir(dir)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	parent, err := checkedDir(filepath.Dir(path))
	if err != nil {
		return err
	}
	if !contained(parent, dir) {
		return fmt.Errorf("output directory outside step directory")
	}
	if err = os.MkdirAll(parent, 0700); err != nil {
		return hide("create output directory", err)
	}
	path = filepath.Join(parent, filepath.Base(path))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return hide("create output", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(data); err != nil {
		return hide("write output", err)
	}
	if err := f.Sync(); err != nil {
		return hide("sync output", err)
	}
	if err := f.Close(); err != nil {
		return hide("close output", err)
	}
	return nil
}

func writeJSON(dir, name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return hide("encode artifact", err)
	}
	return writeFile(dir, name, append(data, '\n'))
}

func writeRows[T any](dir, name string, rows []T) error {
	var data []byte
	for _, row := range rows {
		encoded, err := json.Marshal(row)
		if err != nil {
			return hide("encode row", err)
		}
		data = append(data, encoded...)
		data = append(data, '\n')
	}
	return writeFile(dir, name, data)
}

func loadRows[T any](path string) ([]T, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	var rows []T
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var row T
		if err := dec.Decode(&row); errors.Is(err, io.EOF) {
			return rows, nil
		} else if err != nil {
			return nil, hide("decode rows", err)
		}
		rows = append(rows, row)
	}
}

func outputHashes(dir string) (map[string]string, error) {
	outputs := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return hide("inspect outputs", err)
		}
		if entry.IsDir() {
			if _, checkErr := checkedDir(path); checkErr != nil {
				return checkErr
			}
			return nil
		}
		name, err := filepath.Rel(dir, path)
		if err != nil {
			return hide("resolve output", err)
		}
		if name == "manifest.json" {
			return nil
		}
		data, err := readFile(path)
		if err != nil {
			return err
		}
		outputs[filepath.ToSlash(name)] = digest(data)
		return nil
	})
	return outputs, err
}

func validate(dir string, expected recipe, inputs map[string]*string) error {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("invalid step directory")
	}
	data, err := readFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return err
	}
	var m manifest
	if err = json.Unmarshal(data, &m); err != nil {
		return hide("invalid manifest", err)
	}
	a, err := json.Marshal(expected)
	if err != nil {
		return hide("encode recipe", err)
	}
	b, err := json.Marshal(m.Recipe)
	if err != nil {
		return hide("encode recipe", err)
	}
	if !equalJSON(a, b) || !reflect.DeepEqual(inputs, m.Inputs) {
		return fmt.Errorf("inputs or recipe changed")
	}
	outputs, err := outputHashes(dir)
	if err != nil {
		return err
	}
	if len(outputs) == 0 || !reflect.DeepEqual(outputs, m.Outputs) {
		return fmt.Errorf("output validation failed")
	}
	return nil
}

func equalJSON(a, b []byte) bool {
	var left, right any
	if json.Unmarshal(a, &left) != nil || json.Unmarshal(b, &right) != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
}

func syncDirectory(path string) error {
	path, err := checkedDir(path)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return hide("open output directory", err)
	}
	defer func() { _ = f.Close() }()
	if err := f.Sync(); err != nil {
		return hide("sync output directory", err)
	}
	return nil
}

func seal(dir, target string, r recipe, inputs map[string]*string) error {
	outputs, err := outputHashes(dir)
	if err != nil {
		return err
	}
	if err := writeJSON(dir, "manifest.json", manifest{r, inputs, outputs}); err != nil {
		return err
	}
	var dirs []string
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return hide("inspect output directory", err)
		}
		if entry.IsDir() {
			checked, err := checkedDir(path)
			if err != nil {
				return err
			}
			dirs = append(dirs, checked)
		}
		return nil
	}); err != nil {
		return err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, path := range dirs {
		if err := syncDirectory(path); err != nil {
			return err
		}
	}
	if err := os.Rename(dir, target); err != nil {
		return hide("publish step", err)
	}
	return syncDirectory(filepath.Dir(target))
}
