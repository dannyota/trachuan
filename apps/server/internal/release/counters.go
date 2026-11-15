package release

import (
	"log/slog"
	"time"

	"danny.vn/trachuan/internal/retrieval"
)

func (entry *cacheEntry) startCounters(engine *retrieval.Engine) {
	if entry.stopCounters != nil {
		return
	}
	entry.stopCounters = make(chan struct{})
	entry.countersDone = make(chan struct{})
	go func() {
		defer close(entry.countersDone)
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				entry.logCounters(engine)
			case <-entry.stopCounters:
				return
			}
		}
	}()
}

func (entry *cacheEntry) logCounters(engine *retrieval.Engine) {
	counts := engine.LexicalSources()
	slog.Info("retrieval counters", "release", entry.name, "lexical_sql", counts.SQL, "lexical_postings", counts.Postings, "lexical_reused", counts.Reused, "fallback_searches", counts.FallbackSearches, "fallback_groups", counts.FallbackGroups, "searches", counts.Searches)
}
