package loadrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type runner struct {
	config    Config
	abortSeed uint64
	accounts  []account
	queries   *queries
	keys      []string
	output    *recorder
	client    *http.Client
	cancel    context.CancelFunc
	mu        sync.Mutex
	err       error
}

func Run(ctx context.Context, c Config) error {
	if c.Pages == "" || c.API == "" || c.PublicURL == "" || c.Out == "" {
		return errors.New("pages, api, public-url and out are required")
	}
	if len(c.Phases) == 0 {
		return errors.New("phases are required")
	}
	seen := map[string]bool{}
	for _, p := range c.Phases {
		if p != "A" && p != "B" && p != "C" && p != "D" || seen[p] {
			return errors.New("invalid phases")
		}
		seen[p] = true
	}
	if seen["A"] && c.DurationA <= 0 || seen["C"] && c.DurationC <= 0 || seen["D"] && c.DurationD <= 0 {
		return errors.New("durations must be positive")
	}
	q, err := newQueries(c.Queries)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(c.Out, 0700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(c.Out, "requests.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create request log: %w", err)
	}
	recorder := &recorder{file: f}

	cl := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.API+"/api/v1/public/documents", nil)
	if err != nil {
		_ = f.Close()
		return errors.New("invalid API origin")
	}
	res, err := cl.Do(req)
	if err != nil {
		_ = f.Close()
		return errors.New("summary list request failed")
	}
	var list struct {
		Items []struct {
			Key string `json:"key"`
		} `json:"items"`
	}
	err = json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&list)
	_ = res.Body.Close()
	if err != nil || res.StatusCode != 200 || len(list.Items) == 0 {
		_ = f.Close()
		return errors.New("summary list unavailable or empty")
	}
	keys := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		if item.Key == "" {
			_ = f.Close()
			return errors.New("summary list contains empty key")
		}
		keys = append(keys, item.Key)
	}
	if seen["C"] && len(keys) < 500 {
		_ = f.Close()
		return errors.New("phase C requires 500 summary URLs")
	}
	if len(c.Queries) < requiredQueries(c, len(keys)) {
		_ = f.Close()
		return errors.New("insufficient queries for selected phases")
	}
	accts, err := accounts(ctx, c)
	if err != nil {
		_ = f.Close()
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	run := &runner{config: c, abortSeed: rand.Uint64(), accounts: accts, queries: q, keys: keys, output: recorder, client: cl, cancel: cancel}
	for _, phase := range c.Phases {
		if ctx.Err() != nil {
			break
		}
		run.phase(ctx, phase)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("close request log: %w", err)
	}
	if recorder.err != nil {
		return errors.New("write request log failed")
	}
	if err = writeReport(c.Out, recorder.report()); err != nil {
		return err
	}
	if run.err != nil {
		return run.err
	}
	if ctx.Err() != nil {
		return fmt.Errorf("run interrupted: %w", ctx.Err())
	}
	return nil
}
func (r *runner) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err == nil {
		r.err = err
		r.cancel()
	}
}
func (r *runner) phase(ctx context.Context, phase string) {
	duration := r.config.DurationA
	if phase == "C" {
		duration = r.config.DurationC
	}
	if phase == "D" {
		duration = r.config.DurationD
	}
	phaseCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	launch := func(fn func()) { wg.Add(1); go func() { defer wg.Done(); fn() }() }
	if phase == "B" {
		duration = 0
	}
	launch(func() { schedule(phaseCtx, duration, 2, func(i int) { r.search(ctx, phase, i) }) })
	launch(func() { schedule(phaseCtx, duration, 1, func(i int) { r.reader(ctx, phase, i) }) })
	switch phase {
	case "B":
		schedule(phaseCtx, time.Duration(len(r.keys))*time.Second/8, 8, func(i int) {
			if i < len(r.keys) {
				r.summary(phaseCtx, phase, "summary", i, r.keys[i], 0)
			}
		})
		cancel()
	case "C":
		launch(func() { r.warm(phaseCtx, duration) })
	case "D":
		launch(func() {
			schedule(phaseCtx, duration, 40, func(i int) { r.summary(phaseCtx, phase, "summary", i, r.keys[i%len(r.keys)], 0) })
		})
		launch(func() {
			schedule(phaseCtx, duration, 50, func(i int) { r.summary(phaseCtx, phase, "public_api", i, r.keys[i%len(r.keys)], 0) })
		})
		launch(func() { schedule(phaseCtx, duration, 10, func(i int) { r.summary(phaseCtx, phase, "list", i, "", 0) }) })
		launch(func() { r.activate(phaseCtx, duration) })
	}
	wg.Wait()
}
func (r *runner) activate(ctx context.Context, duration time.Duration) {
	err := runActivations(ctx, duration, []time.Duration{time.Minute, 2 * time.Minute}, func(hookCtx context.Context, target string) error {
		at := time.Now()
		fmt.Printf("activation %s %s\n", target, at.Format(time.RFC3339Nano))
		r.output.activation(at)
		if r.config.Activate == "" {
			return nil
		}
		cmd := exec.CommandContext(hookCtx, "sh", "-c", r.config.Activate+` "$1"`, "loadcheck", target)
		if err := cmd.Run(); err != nil {
			return errors.New("activation command failed")
		}
		return nil
	})
	if err != nil {
		r.fail(err)
	}
}
