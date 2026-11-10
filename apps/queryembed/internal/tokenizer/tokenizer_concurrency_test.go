//go:build ort

package tokenizer

import (
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type nativeCalls struct {
	encode func(string, bool) ([]uint32, []string, error)
	close  func() error
}

func (n *nativeCalls) EncodeErr(text string, special bool) ([]uint32, []string, error) {
	return n.encode(text, special)
}

func (n *nativeCalls) Close() error { return n.close() }

func TestTokenizer_ConcurrentEncodeIsSerialized(t *testing.T) {
	var active atomic.Int32
	var overlap atomic.Bool
	entered := make(chan struct{}, 16)
	release := make(chan struct{})
	tok := &Tokenizer{native: &nativeCalls{
		encode: func(text string, special bool) ([]uint32, []string, error) {
			if active.Add(1) != 1 {
				overlap.Store(true)
			}
			defer active.Add(-1)
			entered <- struct{}{}
			<-release
			if text != "hello world" || !special {
				t.Errorf("native EncodeErr arguments = %q, %v", text, special)
			}
			return []uint32{1, 3, 4, 2}, nil, nil
		},
		close: func() error { return nil },
	}}
	var started, done sync.WaitGroup
	started.Add(16)
	for range 16 {
		done.Go(func() {
			started.Done()
			ids, err := tok.Encode("hello world")
			if err != nil || !reflect.DeepEqual(ids, []int64{1, 3, 4, 2}) {
				t.Errorf("Encode = %v, %v", ids, err)
			}
		})
	}
	started.Wait()
	<-entered
	select {
	case <-entered:
		t.Error("a second native EncodeErr entered while the first was blocked")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	done.Wait()
	if overlap.Load() || active.Load() != 0 {
		t.Error("native EncodeErr calls overlapped or did not finish")
	}
}

func TestTokenizer_CloseWaitsForEncode(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	nativeClosed := make(chan struct{})
	var active atomic.Bool
	var overlap atomic.Bool
	tok := &Tokenizer{native: &nativeCalls{
		encode: func(string, bool) ([]uint32, []string, error) {
			active.Store(true)
			close(entered)
			<-release
			active.Store(false)
			return []uint32{1, 3, 2}, nil, nil
		},
		close: func() error {
			overlap.Store(active.Load())
			close(nativeClosed)
			return nil
		},
	}}
	encoded := make(chan error, 1)
	go func() {
		_, err := tok.Encode("hello")
		encoded <- err
	}()
	<-entered
	closeStarted := make(chan struct{})
	closed := make(chan error, 1)
	go func() {
		close(closeStarted)
		closed <- tok.Close()
	}()
	<-closeStarted
	select {
	case <-nativeClosed:
		t.Error("native Close entered while Encode was blocked")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	if err := <-encoded; err != nil {
		t.Errorf("Encode = %v", err)
	}
	if err := <-closed; err != nil {
		t.Errorf("Close = %v", err)
	}
	select {
	case <-nativeClosed:
	default:
		t.Error("Close did not release the native tokenizer")
	}
	if overlap.Load() {
		t.Error("native Close overlapped Encode")
	}
	if _, err := tok.Encode("hello"); err == nil {
		t.Error("Encode after Close succeeded")
	}
}
