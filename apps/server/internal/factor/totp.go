package factor

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"time"
)

type failure string

func (f failure) Error() string { return string(f) }

const (
	ErrFailed  failure = "factor_failed"
	ErrLimited failure = "factor rate limited"
)

type Limited struct{ Wait time.Duration }

func (e *Limited) Error() string        { return string(ErrLimited) }
func (e *Limited) Is(target error) bool { return target == ErrLimited }

type State struct {
	LastStep                     *int64
	Failures                     int32
	CooldownUntil, LastFailureAt *time.Time
}

func Verify(secret []byte, code string, now time.Time, state *State) error {
	if state.CooldownUntil != nil && state.CooldownUntil.After(now) {
		return &Limited{Wait: state.CooldownUntil.Sub(now)}
	}
	step := now.Unix() / 30
	for candidate := step - 1; candidate <= step+1; candidate++ {
		if candidate < 0 || state.LastStep != nil && candidate <= *state.LastStep {
			continue
		}
		if len(secret) == 20 && len(code) == 6 && subtle.ConstantTimeCompare([]byte(code), []byte(Code(secret, candidate))) == 1 {
			state.LastStep = &candidate
			if state.LastFailureAt != nil && state.LastFailureAt.Before(now.Add(-24*time.Hour)) {
				state.Failures = 0
			}
			return nil
		}
	}
	if state.Failures < 1<<31-1 {
		state.Failures++
	}
	state.LastFailureAt = &now
	if state.Failures >= 5 {
		wait := min(15*time.Minute*time.Duration(1<<min(state.Failures-5, 7)), 24*time.Hour)
		until := now.Add(wait)
		state.CooldownUntil = &until
	}
	return ErrFailed
}

func Code(secret []byte, step int64) string {
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, secret)
	_, _ = mac.Write(counter[:])
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 15
	value := binary.BigEndian.Uint32(digest[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1000000)
}
