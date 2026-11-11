package credential

import (
	"container/list"
	"time"
)

const maxEntries = 10000

type cacheEntry struct {
	hash      string
	principal Principal
	expires   time.Time
	until     time.Time
}

func (v *Verifier) cached(hash string, now time.Time) (cacheEntry, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	e := v.entries[hash]
	if e == nil {
		return cacheEntry{}, false
	}
	entry := e.Value.(cacheEntry)
	if !entry.until.After(now) {
		v.remove(e)
		return cacheEntry{}, false
	}
	return entry, true
}

func (v *Verifier) remember(entry cacheEntry, now time.Time) {
	if !entry.until.After(now) {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.entries == nil {
		v.entries = make(map[string]*list.Element)
	}
	if e := v.entries[entry.hash]; e != nil {
		// Concurrent lookups must not refresh an existing entry's deadline.
		if existing := e.Value.(cacheEntry); existing.until.After(entry.until) {
			e.Value = entry
		}
		return
	}
	if len(v.entries) == maxEntries {
		v.remove(v.oldest.Front())
	}
	v.entries[entry.hash] = v.oldest.PushBack(entry)
}

func (v *Verifier) remove(e *list.Element) {
	delete(v.entries, e.Value.(cacheEntry).hash)
	v.oldest.Remove(e)
}

type touchMark struct {
	hash  string
	until time.Time
}

func (v *Verifier) admitTouch(hash string, now time.Time) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	for e := v.used.Front(); e != nil; e = v.used.Front() {
		mark := e.Value.(touchMark)
		if mark.until.After(now) {
			break
		}
		delete(v.touches, mark.hash)
		v.used.Remove(e)
	}
	if _, exists := v.touches[hash]; exists || len(v.touches) == maxEntries {
		// At capacity, skip new touches rather than evict a live throttle.
		return false
	}
	if v.touches == nil {
		v.touches = make(map[string]time.Time)
	}
	until := now.Add(time.Minute)
	v.touches[hash] = until
	v.used.PushBack(touchMark{hash: hash, until: until})
	return true
}
