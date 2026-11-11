package build

import "time"

type phases map[string]int64

func (p phases) observe(name string, started time.Time) {
	p[name] = time.Since(started).Milliseconds()
}
