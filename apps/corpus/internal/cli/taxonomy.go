package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

func runTaxonomy(_ context.Context, args []string, stdout, stderr io.Writer) int {
	fail := func(err error) int { _, _ = fmt.Fprintln(stderr, err); return 1 }
	if len(args) == 0 {
		return fail(fmt.Errorf("usage: corpus taxonomy <import|check>"))
	}
	switch args[0] {
	case "import":
		opts, err := options(args[1:], map[string]bool{"--package": true, "--mapping": true})
		if err != nil {
			return fail(err)
		}
		if opts["--package"] == "" || opts["--mapping"] == "" {
			return fail(fmt.Errorf("--package and --mapping are required"))
		}
		pkg, err := taxonomy.ReadPackage(opts["--package"])
		if err != nil {
			return fail(err)
		}
		var prev taxonomy.Mapping
		data, err := os.ReadFile(opts["--mapping"])
		if err == nil {
			prev, err = taxonomy.ReadMapping(data)
			if err == nil {
				err = taxonomy.Validate(prev, nil, nil)
			}
		} else if errors.Is(err, os.ErrNotExist) {
			err = nil
		}
		if err != nil {
			return fail(err)
		}
		m, err := taxonomy.Import(pkg, prev)
		if err != nil {
			return fail(err)
		}
		if err = taxonomy.Validate(m, nil, nil); err != nil {
			return fail(err)
		}
		if err = writeMapping(opts["--mapping"], m.Encode()); err != nil {
			return fail(err)
		}
		counts := map[int]map[string]int{}
		for _, n := range m.Nodes {
			if counts[n.Level] == nil {
				counts[n.Level] = map[string]int{}
			}
			counts[n.Level][n.Status]++
		}
		for level := 1; level <= 4; level++ {
			_, _ = fmt.Fprintf(stdout, "level %d active %d retired %d\n", level, counts[level]["active"], counts[level]["retired"])
		}
		return 0
	case "check":
		opts, err := options(args[1:], map[string]bool{"--mapping": true, "--custom": true, "--overrides": true, "--package": true})
		if err != nil {
			return fail(err)
		}
		if opts["--mapping"] == "" || opts["--custom"] == "" || opts["--overrides"] == "" {
			return fail(fmt.Errorf("--mapping, --custom and --overrides are required"))
		}
		data, err := os.ReadFile(opts["--mapping"])
		if err != nil {
			return fail(err)
		}
		m, err := taxonomy.ReadMapping(data)
		if err != nil {
			return fail(err)
		}
		data, err = os.ReadFile(opts["--custom"])
		if err != nil {
			return fail(err)
		}
		custom, err := taxonomy.ReadCustom(data)
		if err != nil {
			return fail(err)
		}
		data, err = os.ReadFile(opts["--overrides"])
		if err != nil {
			return fail(err)
		}
		overrides, err := taxonomy.ReadOverrides(data)
		if err != nil {
			return fail(err)
		}
		if err = taxonomy.Validate(m, custom, overrides); err != nil {
			return fail(err)
		}
		if opts["--package"] != "" {
			pkg, err := taxonomy.ReadPackage(opts["--package"])
			if err != nil {
				return fail(err)
			}
			if pkg.SHA256 != m.PackageSHA256 {
				return fail(fmt.Errorf("package hash mismatch"))
			}
			ids := map[string]bool{}
			for _, n := range pkg.ChuDe {
				ids[n.GUID] = true
			}
			for _, n := range pkg.DeMuc {
				ids[n.GUID] = true
			}
			for _, n := range pkg.Tree {
				ids[n.GUID] = true
			}
			for _, n := range m.Nodes {
				if n.Status == "active" && !ids[n.GUID] {
					return fail(fmt.Errorf("active GUID %s missing from package", n.GUID))
				}
			}
		}
		_, _ = fmt.Fprintln(stdout, "taxonomy valid")
		return 0
	default:
		return fail(fmt.Errorf("unknown taxonomy command %q", args[0]))
	}
}
func writeMapping(path string, data []byte) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), ".mapping-*")
	if err != nil {
		return fmt.Errorf("create mapping temporary file: %w", err)
	}
	name := f.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write mapping: %w", err)
	}
	if err = f.Chmod(0644); err != nil {
		_ = f.Close()
		return fmt.Errorf("chmod mapping: %w", err)
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync mapping: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("close mapping: %w", err)
	}
	if err = os.Rename(name, path); err != nil {
		return fmt.Errorf("rename mapping: %w", err)
	}
	return nil
}
