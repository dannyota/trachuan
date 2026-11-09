package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/dannyota/traquydinh/apps/corpus/internal/config"
	"github.com/dannyota/traquydinh/apps/corpus/internal/run"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"
	"gopkg.in/yaml.v3"
)

func dependencies(ctx context.Context, needSources bool, cc string) (run.Deps, func(), error) {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return run.Deps{}, nil, fmt.Errorf("load configuration: %w", err)
	}
	if needSources && cfg.CheckpointDir == "" {
		return run.Deps{}, nil, fmt.Errorf("CORPUS_CHECKPOINT_DIR is required")
	}
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return run.Deps{}, nil, fmt.Errorf("open workshop: %w", err)
	}
	temporalClient, err := client.Dial(client.Options{HostPort: cfg.TemporalAddress, Namespace: cfg.Namespace})
	if err != nil {
		db.Close()
		return run.Deps{}, nil, fmt.Errorf("connect Temporal: %w", err)
	}
	closeDeps := func() { temporalClient.Close(); db.Close() }
	stages, recipes := compiledStages()
	d := run.Deps{DB: db, Temporal: temporalClient, Stages: stages, Recipes: recipes}
	if needSources {
		if !regexp.MustCompile(`^[a-z]{2}$`).MatchString(cc) {
			closeDeps()
			return run.Deps{}, nil, fmt.Errorf("invalid country %q", cc)
		}
		data, readErr := os.ReadFile("sources/" + cc + ".yaml")
		if readErr != nil {
			closeDeps()
			return run.Deps{}, nil, fmt.Errorf("read source list: %w", readErr)
		}
		var source any
		if yamlErr := yaml.Unmarshal(data, &source); yamlErr != nil {
			closeDeps()
			return run.Deps{}, nil, fmt.Errorf("parse source list: %w", yamlErr)
		}
		d.SourceList, err = json.Marshal(source)
		if err != nil {
			closeDeps()
			return run.Deps{}, nil, fmt.Errorf("encode source list: %w", err)
		}
	}
	return d, closeDeps, nil
}

func options(args []string, allowed map[string]bool) (map[string]string, error) {
	values := make(map[string]string)
	for i := 0; i < len(args); i++ {
		name := args[i]
		hasValue, ok := allowed[name]
		if !ok {
			return nil, fmt.Errorf("unknown option %q", name)
		}
		if _, exists := values[name]; exists {
			return nil, fmt.Errorf("duplicate option %q", name)
		}
		if hasValue {
			i++
			if i == len(args) {
				return nil, fmt.Errorf("missing value for %s", name)
			}
			values[name] = args[i]
		} else {
			values[name] = "true"
		}
	}
	return values, nil
}

func lifecycle(ctx context.Context, name string, args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		_, _ = fmt.Fprintf(stderr, "usage: corpus %s <cc> <rel> [options]\n", name)
		return 2
	}
	cc, rel := args[0], args[1]
	allowed := map[string]bool{}
	switch name {
	case "start":
		allowed["--until"] = true
		allowed["--previous-manifest"] = true
	case "recover":
		allowed["--terminate"] = false
	case "accept-gaps":
		allowed["--kind"] = true
		allowed["--note"] = true
	}
	opts, err := options(args[2:], allowed)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	if name == "accept-gaps" && opts["--kind"] == "" {
		_, _ = fmt.Fprintln(stderr, "--kind is required")
		return 2
	}
	d, closeDeps, err := dependencies(ctx, name == "start", cc)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer closeDeps()
	var result any
	switch name {
	case "start":
		err = run.Start(ctx, d, cc, rel, opts["--until"], opts["--previous-manifest"])
	case "status":
		result, err = run.Status(ctx, d, cc, rel)
	case "cancel":
		err = run.Cancel(ctx, d, cc, rel)
	case "recover":
		err = run.Recover(ctx, d, cc, rel, opts["--terminate"] == "true")
	case "gaps":
		result, err = run.ListGaps(ctx, d, cc, rel)
	case "accept-gaps":
		result, err = run.AcceptGaps(ctx, d, cc, rel, opts["--kind"], opts["--note"])
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if result != nil {
		if encodeErr := json.NewEncoder(stdout).Encode(result); encodeErr != nil {
			_, _ = fmt.Fprintln(stderr, encodeErr)
			return 1
		}
	}
	return 0
}
