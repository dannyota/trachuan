package build

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/unix"
)

func Run(ctx context.Context, rel string, out io.Writer) error {
	if !regexp.MustCompile(`^[0-9]{8}(_[0-9]{1,2})?$`).MatchString(rel) {
		return fmt.Errorf("invalid release id")
	}
	source, work, err := prepare()
	if err != nil {
		return err
	}
	base, err := checkedDir(filepath.Join(work, rel))
	if err != nil {
		return err
	}
	if !contained(base, work) {
		return fmt.Errorf("release directory outside work directory")
	}
	if err = makeDirectory(base); err != nil {
		return err
	}
	fd, err := unix.Open(filepath.Join(base, ".lock"), unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return hide("open release lock", err)
	}
	defer func() { _ = unix.Close(fd) }()
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return hide("release locked", err)
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return hide("read release directory", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tmp-") {
			if err = removeTemp(filepath.Join(base, entry.Name())); err != nil {
				return err
			}
		}
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return hide("create run id", err)
	}
	for _, step := range []string{"snapshot", "parse", "relate", "chunk"} {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%s: %w", step, err)
		}
		if err := runStep(ctx, base, step, fmt.Sprintf("%x", id), source, out); err != nil {
			return fmt.Errorf("%s: %w", step, err)
		}
	}
	return nil
}

func stepRecipe(step string) recipe {
	params := map[string]any{}
	switch step {
	case "parse":
		params["schema"] = "controls/1"
	case "relate":
		params["schema"] = "edges/1"
	case "chunk":
		params["schema"] = "chunks/1"
		params["limit"] = 512
		params["prefix_format_version"] = 1
	}
	return recipe{Name: step, Version: 1, Parameters: params}
}

func stepInputs(base, step string) (map[string]*string, error) {
	inputs := map[string]*string{}
	deps := []string{"snapshot"}
	if step == "relate" || step == "chunk" {
		deps = append(deps, "parse")
	}
	for _, dep := range deps {
		name := dep + "/manifest.json"
		data, err := readFile(filepath.Join(base, name))
		if err != nil {
			return nil, err
		}
		hash := digest(data)
		inputs[name] = &hash
	}
	return inputs, nil
}

func runStep(ctx context.Context, base, step, runID, source string, out io.Writer) error {
	target, err := checkedDir(filepath.Join(base, step))
	if err != nil {
		return err
	}
	if !contained(target, base) {
		return fmt.Errorf("step directory outside release directory")
	}
	info, err := os.Lstat(target)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return hide("inspect step", err)
	}
	if exists && !info.IsDir() {
		return fmt.Errorf("invalid step directory")
	}
	err = nil
	var inputs map[string]*string
	if step == "snapshot" && exists {
		inputs, err = snapshotInputs(target)
	} else if step != "snapshot" {
		inputs, err = stepInputs(base, step)
	}
	if err != nil {
		return err
	}
	r := stepRecipe(step)
	if exists {
		if err = validate(target, r, inputs); err != nil {
			return err
		}
		if _, err = fmt.Fprintf(out, "%s skipped\n", step); err != nil {
			return hide("write progress", err)
		}
		return nil
	}
	tmp, err := checkedDir(filepath.Join(base, ".tmp-"+step+"-"+runID))
	if err != nil {
		return err
	}
	if !contained(tmp, base) {
		return fmt.Errorf("temporary directory outside release directory")
	}
	if err = os.Mkdir(tmp, 0700); err != nil {
		return hide("create step directory", err)
	}
	switch step {
	case "snapshot":
		inputs, err = snapshot(ctx, tmp, source)
	case "parse":
		err = parseStep(ctx, base, tmp)
	case "relate":
		err = relateStep(ctx, base, tmp)
	case "chunk":
		err = chunkStep(ctx, base, tmp)
	}
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := seal(tmp, target, r, inputs); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "%s complete\n", step); err != nil {
		return hide("write progress", err)
	}
	return nil
}
