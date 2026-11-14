package factor

import (
	"errors"
	"testing"
	"time"
)

func TestVerify_WindowAndReplay(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(59, 0)
	for _, tc := range []struct {
		code string
		last *int64
		want int64
		ok   bool
	}{
		{"755224", nil, 0, true}, {"287082", nil, 1, true}, {"359152", nil, 2, true},
		{"969429", nil, 0, false}, {"287082", new(int64(1)), 0, false},
		{"359152", new(int64(1)), 2, true}, {"28708", nil, 0, false}, {" 287082", nil, 0, false},
	} {
		state := State{LastStep: tc.last}
		err := Verify(secret, tc.code, now, &state)
		if (err == nil) != tc.ok {
			t.Fatalf("code %q: %v", tc.code, err)
		}
		if tc.ok && (state.LastStep == nil || *state.LastStep != tc.want) {
			t.Fatal("wrong accepted step")
		}
	}
}

func TestVerify_CooldownAndFailureReset(t *testing.T) {
	now := time.Unix(1800000000, 0)
	state := State{}
	waits := []time.Duration{0, 0, 0, 0, 15 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour, 4 * time.Hour, 8 * time.Hour, 16 * time.Hour, 24 * time.Hour, 24 * time.Hour}
	for i, wait := range waits {
		if err := Verify([]byte("12345678901234567890"), "wrong", now, &state); !errors.Is(err, ErrFailed) {
			t.Fatal(err)
		}
		if state.Failures != int32(i+1) {
			t.Fatal("failure not counted")
		}
		if wait > 0 {
			if state.CooldownUntil == nil || state.CooldownUntil.Sub(now) != wait {
				t.Fatalf("failure %d cooldown", i+1)
			}
			before := state.Failures
			if err := Verify(nil, "287082", now, &state); !errors.Is(err, ErrLimited) || state.Failures != before {
				t.Fatal("cooldown checked code", err)
			}
			now = now.Add(wait)
		}
	}
	for i := len(waits); i < 35; i++ {
		if err := Verify([]byte("12345678901234567890"), "wrong", now, &state); !errors.Is(err, ErrFailed) || state.Failures != int32(i+1) {
			t.Fatal("failure counter stopped incrementing", state.Failures)
		}
		now = now.Add(24 * time.Hour)
	}
	for _, age := range []time.Duration{24 * time.Hour, 24*time.Hour + time.Second} {
		last := time.Unix(59, 0).Add(-age)
		state = State{Failures: 4, LastFailureAt: &last}
		if err := Verify([]byte("12345678901234567890"), "287082", time.Unix(59, 0), &state); err != nil {
			t.Fatal(err)
		}
		want := int32(4)
		if age > 24*time.Hour {
			want = 0
		}
		if state.Failures != want {
			t.Fatal("wrong failure reset boundary")
		}
	}
}
