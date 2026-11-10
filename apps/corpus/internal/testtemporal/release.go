package testtemporal

import (
	"fmt"
	"sync/atomic"
	"time"
)

var counter atomic.Uint64

// Release returns a release name unique within the test binary.
func Release() string {
	n := counter.Add(1) - 1
	if n >= 36500 {
		panic("exhausted test releases in 2099")
	}
	date := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(n%365)).Format("20060102")
	if n < 365 {
		return date
	}
	return fmt.Sprintf("%s_%d", date, n/365)
}
