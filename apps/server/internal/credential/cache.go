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

type lookupFlight struct {
	hash    string
	revoked bool
}

// Evict removes a cached credential and invalidates lookups already in flight.
func (v *Verifier) Evict(hash string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if e := v.entries[hash]; e != nil {
		v.remove(e)
	}
	for flight := range v.flights {
		if flight.hash == hash {
			flight.revoked = true
		}
	}
}

func (v *Verifier) startLookup(hash string, now time.Time) (cacheEntry, *lookupFlight, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if e := v.entries[hash]; e != nil {
		entry := e.Value.(cacheEntry)
		if entry.until.After(now) {
			return entry, nil, true
		}
		v.remove(e)
	}
	flight := &lookupFlight{hash: hash}
	if v.flights == nil {
		v.flights = make(map[*lookupFlight]struct{})
	}
	v.flights[flight] = struct{}{}
	return cacheEntry{}, flight, false
}

func (v *Verifier) finishLookup(flight *lookupFlight) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.flights, flight)
}

func (v *Verifier) remember(entry cacheEntry, now time.Time, flight *lookupFlight) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if flight.revoked {
		return false
	}
	if !entry.until.After(now) {
		return true
	}
	if v.entries == nil {
		v.entries = make(map[string]*list.Element)
	}
	if e := v.entries[entry.hash]; e != nil {
		if existing := e.Value.(cacheEntry); existing.until.After(entry.until) {
			e.Value = entry
		}
		return true
	}
	if len(v.entries) == maxEntries {
		v.remove(v.oldest.Front())
	}
	v.entries[entry.hash] = v.oldest.PushBack(entry)
	return true
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
