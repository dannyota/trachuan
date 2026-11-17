package publicsummary

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type Lease struct {
	Release Release
	manager *Manager
	entry   *entry
	once    sync.Once
}

func (l *Lease) Done() {
	l.once.Do(func() {
		m := l.manager
		m.mu.Lock()
		defer m.mu.Unlock()
		l.entry.refs--
		if l.entry.retired && l.entry.refs == 0 {
			m.closeEntry(l.entry)
		}
	})
}

func (l *Lease) queryContext(ctx context.Context) (context.Context, func(), error) {
	l.manager.mu.Lock()
	defer l.manager.mu.Unlock()
	if l.entry.retired {
		return nil, nil, ErrBusy
	}
	if l.entry.pool == nil {
		return nil, nil, ErrNotFound
	}
	select {
	case l.entry.slot <- struct{}{}:
	default:
		return nil, nil, ErrBusy
	}
	work, cancel := context.WithTimeout(ctx, 2*time.Second)
	stop := context.AfterFunc(l.entry.ctx, cancel)
	return work, func() { stop(); cancel(); <-l.entry.slot }, nil
}

func (l *Lease) Row(ctx context.Context, key string) (Row, error) {
	rawKey, _ := json.Marshal([]string{l.Release.ID, key})
	hash := sha256.Sum256(rawKey)
	l.manager.mu.Lock()
	if l.entry.retired {
		l.manager.mu.Unlock()
		return Row{}, ErrBusy
	}
	raw, hit := l.entry.cache.Get(hash)
	l.manager.mu.Unlock()
	if hit {
		var row Row
		if err := json.Unmarshal(raw, &row); err == nil {
			return row, nil
		}
	}
	work, done, err := l.queryContext(ctx)
	if err != nil {
		return Row{}, err
	}
	defer done()
	row, err := l.entry.pool.Row(work, key)
	if err != nil {
		return Row{}, err
	}
	if err = work.Err(); err != nil {
		return Row{}, fmt.Errorf("public summary query: %w", err)
	}
	raw, err = json.Marshal(row)
	if err != nil {
		return Row{}, fmt.Errorf("cache public summary: %w", err)
	}
	l.manager.mu.Lock()
	if !l.entry.retired {
		l.entry.cache.Put(hash, raw)
	}
	l.manager.mu.Unlock()
	return row, nil
}

func (l *Lease) List(ctx context.Context) ([]Item, error) {
	work, done, err := l.queryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	items, err := l.entry.pool.List(work)
	if err != nil {
		return nil, err
	}
	if err = work.Err(); err != nil {
		return nil, fmt.Errorf("public list query: %w", err)
	}
	return items, nil
}
