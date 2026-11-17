package publicsummary

import "time"

func (m *Manager) openEntry(e *entry) {
	var pool Pool
	var err error
	if e.release.Projection == 1 {
		select {
		case <-e.ctx.Done():
			err = e.ctx.Err()
		case m.resources <- struct{}{}:
			e.reserved = true
			if err = e.ctx.Err(); err == nil {
				pool, err = m.open(e.ctx, e.release.Database)
			}
		}
	}
	m.mu.Lock()
	e.pool, e.openErr = pool, err
	close(e.ready)
	retired := e.retired || m.closed || err != nil
	m.mu.Unlock()
	if retired {
		m.closeEntry(e)
	}
}

func (m *Manager) retireLocked() {
	old := m.active
	if old == nil {
		return
	}
	old.retired = true
	old.cache.Purge()
	m.active = nil
	if old.refs == 0 {
		m.closeEntry(old)
		return
	}
	m.after(5*time.Second, func() { m.closeEntry(old) })
}

func (m *Manager) closeEntry(e *entry) {
	e.close.Do(func() {
		e.cancel()
		go func() {
			<-e.ready
			if e.pool != nil {
				e.pool.Close()
			}
			if e.reserved {
				<-m.resources
			}
			m.mu.Lock()
			delete(m.entries, e)
			m.mu.Unlock()
		}()
	})
}

func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	m.active = nil
	for e := range m.entries {
		e.retired = true
		e.cache.Purge()
		m.closeEntry(e)
	}
	m.mu.Unlock()
	m.cancel()
}
