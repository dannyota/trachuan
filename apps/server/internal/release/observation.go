package release

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const authQueryDeadline = 2 * time.Second

func (r *Resolver) resolveName(ctx context.Context, country, rel string) (string, string, uint64, error) {
	if err := ctx.Err(); err != nil {
		return "", "", 0, err
	}
	if r.Auth == nil || r.Clock == nil {
		return "", "", 0, errors.New("missing release resolver dependencies")
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return "", "", 0, ErrExpired
	}
	if r.observation == nil {
		r.observation = make(chan struct{}, 1)
	}
	observation := r.observation
	r.mu.Unlock()
	timeout := r.QueryTimeout
	if timeout <= 0 || timeout > authQueryDeadline {
		timeout = authQueryDeadline
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case observation <- struct{}{}:
	case <-ctx.Done():
		return "", "", 0, ctx.Err()
	}
	defer func() { <-observation }()
	active, err := r.Auth.GetActiveRelease(ctx, country)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return "", "", 0, fmt.Errorf("get active release: %w", err)
	}
	if active != "" && !validDBName(country, active) {
		return "", "", 0, errors.New("invalid active database name")
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return "", "", 0, ErrExpired
	}
	if r.active == nil {
		r.active = make(map[string]string)
	}
	previous, observed := r.active[country]
	r.active[country] = active
	for name, entry := range r.entries {
		if entry.country == country && name != active {
			delete(r.entries, name)
			entry.retired = true
			if entry.engine != nil {
				entry.engine.Retire()
			}
			if !entry.opening && entry.leases == 0 {
				r.cleanupLocked(entry)
			}
		}
	}
	var notify func(string, uint64)
	if r.OnActivate != nil && (!observed || previous != active) {
		r.generation++
		notify = r.OnActivate
	}
	generation := r.generation
	allowed := active != "" && (rel == "" || active == "corpus_"+country+"_"+rel)
	r.mu.Unlock()
	release := strings.TrimPrefix(active, "corpus_"+country+"_")
	if notify != nil {
		notify(release, generation)
	}
	if !allowed {
		return "", "", 0, ErrExpired
	}
	return active, release, generation, nil
}
