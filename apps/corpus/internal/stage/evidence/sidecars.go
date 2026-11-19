package evidence

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func (s *Stage) summaryFor(pool *pgxpool.Pool) summarySource {
	if s.summary != nil {
		return s.summary(pool)
	}
	return sqlRelease{q: pool}
}

func writeFile(path string, write func(io.Writer) error) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}
	writeErr := write(f)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", filepath.Base(path), closeErr)
	}
	return nil
}

func writeSidecars(ctx context.Context, dir, rel string, inputs frozenInputs, m members, src summarySource) error {
	identity := filepath.Join(dir, identityFile)
	if err := writeFile(identity, func(w io.Writer) error {
		return writeIdentity(ctx, w, src, m, inputs.identityBindings(rel))
	}); err != nil {
		return err
	}
	return writeFile(filepath.Join(dir, changesFile), func(w io.Writer) error {
		return writeChanges(w, changeInputs{current: identity, predecessor: inputs.predecessor.identityPath, members: m, bindings: inputs.changesBindings(rel)})
	})
}

func readHeader(path string, out any) error {
	f, err := os.Open(path)
	if err != nil {
		return refuse("open %s: %v", filepath.Base(path), err)
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1<<12), lineLimit+1)
	if !scanner.Scan() {
		return refuse("%s has no header", filepath.Base(path))
	}
	if err = strictDecode(scanner.Bytes(), out); err != nil {
		return refuse("%s header: %v", filepath.Base(path), err)
	}
	return nil
}

func manifestDigest(dir, name string) (string, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		return "", false, fmt.Errorf("read evidence manifest: %w", err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if len(line) > 66 && line[64:66] == "  " && line[66:] == name {
			return line[:64], true, nil
		}
	}
	return "", false, nil
}

func predecessorIdentity(dir, rel, reportSHA256 string) (string, string, error) {
	sum, ok, err := manifestDigest(dir, identityFile)
	if err != nil {
		return "", "", err
	}
	if !ok {
		return "", "", refuse("predecessor %s evidence has no identity.jsonl", rel)
	}
	path := filepath.Join(dir, identityFile)
	var header identityHeader
	if err = readHeader(path, &header); err != nil {
		return "", "", err
	}
	if header.Schema != summarySchema {
		return "", "", refuse("predecessor %s identity schema %d is not %d", rel, header.Schema, summarySchema)
	}
	if header.Bindings.Rel != rel || header.Bindings.ReportSHA256 != hashPrefix+reportSHA256 {
		return "", "", refuse("predecessor %s identity does not bind the frozen previous report", rel)
	}
	return path, sum, nil
}

func checkSidecars(dir, rel string, inputs frozenInputs) error {
	var identity identityHeader
	if err := readHeader(filepath.Join(dir, identityFile), &identity); err != nil {
		return err
	}
	if identity.Schema != summarySchema || identity.Bindings != inputs.identityBindings(rel) {
		return errors.New("identity summary does not bind the run")
	}
	var changes changesHeader
	if err := readHeader(filepath.Join(dir, changesFile), &changes); err != nil {
		return err
	}
	want := changesHeader{Schema: summarySchema, Bindings: inputs.changesBindings(rel), Baseline: inputs.predecessor.identityPath == ""}
	if changes != want {
		return errors.New("changes summary does not bind the run")
	}
	return nil
}
