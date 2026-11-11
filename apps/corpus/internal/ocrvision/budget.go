package ocrvision

import "sync/atomic"

type budget struct {
	limit int64
	spent atomic.Int64
}

func newBudget(limit int) *budget { return &budget{limit: int64(limit)} }
func (b *budget) used() int       { return int(b.spent.Load()) }
func (b *budget) remaining() int  { return int(b.limit - b.spent.Load()) }
func (b *budget) debit() bool {
	for {
		used := b.spent.Load()
		if used >= b.limit {
			return false
		}
		if b.spent.CompareAndSwap(used, used+1) {
			return true
		}
	}
}
