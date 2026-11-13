package release

func (r *Resolver) cleanupLocked(entry *cacheEntry) <-chan struct{} {
	if r.cleanups == nil {
		r.cleanups = make(map[*cacheEntry]chan struct{})
	}
	if done, exists := r.cleanups[entry]; exists {
		return done
	}
	done := make(chan struct{})
	r.cleanups[entry] = done
	go func() {
		entry.close()
		r.mu.Lock()
		close(done)
		delete(r.cleanups, entry)
		r.mu.Unlock()
	}()
	return done
}
