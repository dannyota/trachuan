package discover

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"golang.org/x/time/rate"
)

type centralTransport func(*http.Request) (*http.Response, error)

func (f centralTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCentralPass_EightPagesInFlight(t *testing.T) {
	dir := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		release := make([]chan struct{}, 10)
		for i := range release {
			release[i] = make(chan struct{})
		}
		var active, peak atomic.Int32
		transport := centralTransport(func(r *http.Request) (*http.Response, error) {
			var q struct {
				Page int `json:"pageNumber"`
			}
			if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
				return nil, err
			}
			n := active.Add(1)
			defer active.Add(-1)
			for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
			}
			if q.Page > 9 {
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"total":9000,"items":[]}}`)), Request: r}, nil
			}
			if q.Page > 1 {
				<-release[q.Page]
			}
			body := fmt.Sprintf(`{"success":true,"data":{"total":9000,"items":[{"id":"%d"}]}}`, q.Page)
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		})
		client := vbpl.NewWithBase("https://example.test", "test", vbpl.WithTransport(transport), vbpl.WithLimits(float64(rate.Inf), 16))
		type outcome struct {
			ids []string
			err error
		}
		done := make(chan outcome, 1)
		go func() {
			result, _, err := New(client).centralPass(context.Background(), dir, 1, "test", json.RawMessage(`{}`))
			ids := make([]string, len(result.Items))
			for i, item := range result.Items {
				ids[i] = item.SourceID
			}
			done <- outcome{ids, err}
		}()
		synctest.Wait()
		got := peak.Load()
		for i := 9; i >= 2; i-- {
			close(release[i])
			synctest.Wait()
		}
		out := <-done
		if got != 8 || peak.Load() > 8 {
			t.Fatalf("peak in flight = %d, want 8", got)
		}
		if out.err != nil {
			t.Fatal(out.err)
		}
		for i, id := range out.ids {
			if id != fmt.Sprint(i+1) {
				t.Fatalf("items = %v", out.ids)
			}
		}
		if len(out.ids) != 9 {
			t.Fatalf("items = %v", out.ids)
		}
	})
}

func TestCentralPass_LowerFailureWinsAndCancelsHigher(t *testing.T) {
	dir := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		release := make([]chan struct{}, 10)
		cancelled := make([]chan struct{}, 10)
		for i := range release {
			release[i] = make(chan struct{})
			cancelled[i] = make(chan struct{})
		}
		transport := centralTransport(func(r *http.Request) (*http.Response, error) {
			var q struct {
				Page int `json:"pageNumber"`
			}
			if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
				return nil, err
			}
			if q.Page > 1 {
				select {
				case <-release[q.Page]:
				case <-r.Context().Done():
					close(cancelled[q.Page])
					return nil, r.Context().Err()
				}
			}
			status := 200
			if q.Page == 3 {
				status = 404
			}
			if q.Page == 5 {
				status = 400
			}
			body := fmt.Sprintf(`{"success":true,"data":{"total":9000,"items":[{"id":"%d"}]}}`, q.Page)
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		})
		client := vbpl.NewWithBase("https://example.test", "test", vbpl.WithTransport(transport), vbpl.WithLimits(float64(rate.Inf), 16))
		done := make(chan error, 1)
		go func() {
			_, _, err := New(client).centralPass(context.Background(), dir, 1, "test", json.RawMessage(`{}`))
			done <- err
		}()
		synctest.Wait()
		close(release[5])
		synctest.Wait()
		for _, i := range []int{6, 7, 8, 9} {
			select {
			case <-cancelled[i]:
			default:
				t.Errorf("page %d was not cancelled", i)
			}
		}
		for _, i := range []int{2, 3, 4} {
			select {
			case <-cancelled[i]:
				t.Errorf("page %d cancelled too early", i)
			default:
			}
		}
		close(release[2])
		close(release[3])
		synctest.Wait()
		err := <-done
		var status *vbpl.Failure
		if !errors.As(err, &status) || status.HTTPStatus != 404 {
			t.Fatalf("error = %v, want page 3 status 404", err)
		}
		select {
		case <-cancelled[4]:
		default:
			t.Error("page 4 was not cancelled")
		}
	})
}

func TestCentral_RecordedFixturesMatchSequentialArtifact(t *testing.T) {
	pages := make([]string, 3)
	for i := 1; i <= 2; i++ {
		raw := scopeFixture(t, fmt.Sprintf("list-central-size5-page%d.json", i))
		pages[i] = strings.Replace(raw, `"total":57951`, `"total":10`, 1)
		if pages[i] == raw {
			t.Fatal("fixture total not replaced")
		}
	}
	transport := centralTransport(func(r *http.Request) (*http.Response, error) {
		var q struct {
			Page int `json:"pageNumber"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			return nil, err
		}
		body := `{"success":true,"data":{"total":10,"items":[]}}`
		if q.Page <= 2 {
			body = pages[q.Page]
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	client := vbpl.NewWithBase("https://example.test", "test", vbpl.WithTransport(transport), vbpl.WithLimits(float64(rate.Inf), 16))
	s := New(client)
	sequential, concurrent := t.TempDir(), t.TempDir()
	rules := source.VBPLSources{SelectionVersion: 1, SweepAgencies: []string{"62"}, Keywords: []string{"ngân hàng"}}
	if err := s.centralSequential(context.Background(), sequential, rules); err != nil {
		t.Fatal(err)
	}
	if err := s.central(context.Background(), concurrent, rules); err != nil {
		t.Fatal(err)
	}
	compare := func(root string) map[string][]byte {
		files := map[string][]byte{}
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files[relative] = data
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return files
	}
	if !reflect.DeepEqual(compare(sequential), compare(concurrent)) {
		t.Fatal("artifact differs from sequential output")
	}
}

func TestCentralPass_RateLimitBackoffDoesNotBlockOtherPages(t *testing.T) {
	dir := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		var attempts, others atomic.Int32
		var first, second time.Time
		transport := centralTransport(func(r *http.Request) (*http.Response, error) {
			var q struct {
				Page int `json:"pageNumber"`
			}
			if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
				return nil, err
			}
			status := 200
			if q.Page == 2 {
				if attempts.Add(1) == 1 {
					first = time.Now()
					status = 429
				} else {
					second = time.Now()
				}
			} else if q.Page > 2 && q.Page <= 9 {
				others.Add(1)
				<-release
			}
			body := fmt.Sprintf(`{"success":true,"data":{"total":9000,"items":[{"id":"%d"}]}}`, q.Page)
			if q.Page > 9 {
				body = `{"success":true,"data":{"total":9000,"items":[]}}`
			}
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		})
		client := vbpl.NewWithBase("https://example.test", "test", vbpl.WithTransport(transport), vbpl.WithLimits(float64(rate.Inf), 16), vbpl.WithBackoffs(2*time.Second, 8*time.Second, 30*time.Second))
		done := make(chan error, 1)
		go func() {
			_, _, err := New(client).centralPass(context.Background(), dir, 1, "test", json.RawMessage(`{}`))
			done <- err
		}()
		synctest.Wait()
		if others.Load() != 7 {
			t.Errorf("other pages started = %d, want 7", others.Load())
		}
		close(release)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if attempts.Load() != 2 || second.Sub(first) < 2*time.Second {
			t.Fatalf("attempts=%d delay=%s", attempts.Load(), second.Sub(first))
		}
	})
}

func TestCentralPass_PauseCancelsAllPages(t *testing.T) {
	dir := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		var started, cancelled atomic.Int32
		transport := centralTransport(func(r *http.Request) (*http.Response, error) {
			var q struct {
				Page int `json:"pageNumber"`
			}
			if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
				return nil, err
			}
			status := 200
			if q.Page > 1 {
				started.Add(1)
				if q.Page == 5 {
					<-release
					status = 403
				} else {
					<-r.Context().Done()
					cancelled.Add(1)
					return nil, r.Context().Err()
				}
			}
			body := fmt.Sprintf(`{"success":true,"data":{"total":9000,"items":[{"id":"%d"}]}}`, q.Page)
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		})
		client := vbpl.NewWithBase("https://example.test", "test", vbpl.WithTransport(transport), vbpl.WithLimits(float64(rate.Inf), 16), vbpl.WithGate(sourcegate.NewMemory()))
		ctx := sourcegate.WithRunID(sourcegate.WithActivity(context.Background()), 1)
		done := make(chan error, 1)
		go func() { _, _, err := New(client).centralPass(ctx, dir, 1, "test", json.RawMessage(`{}`)); done <- err }()
		synctest.Wait()
		if started.Load() != 8 {
			t.Errorf("started = %d", started.Load())
		}
		close(release)
		err := <-done
		if !errors.Is(err, sourcegate.ErrSourcePaused) || cancelled.Load() != 7 {
			t.Fatalf("error=%v cancelled=%d", err, cancelled.Load())
		}
	})
}

func TestCentralPages_PauseCancelsWhileCallerIsBusy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		cancelled := make(chan struct{})
		transport := centralTransport(func(r *http.Request) (*http.Response, error) {
			var q struct {
				Page int `json:"pageNumber"`
			}
			if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
				return nil, err
			}
			if q.Page == 2 {
				close(started)
				<-r.Context().Done()
				close(cancelled)
				return nil, r.Context().Err()
			}
			<-started
			return &http.Response{StatusCode: 403, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		})
		client := vbpl.NewWithBase("https://example.test", "test", vbpl.WithTransport(transport), vbpl.WithLimits(float64(rate.Inf), 16), vbpl.WithGate(sourcegate.NewMemory()))
		ctx := sourcegate.WithRunID(sourcegate.WithActivity(context.Background()), 1)
		pages := newCentralPages(ctx, client)
		pages.start(2)
		pages.start(3)
		synctest.Wait()
		didCancel := false
		select {
		case <-cancelled:
			didCancel = true
		default:
		}
		_ = pages.close()
		if !didCancel {
			t.Fatal("pause did not cancel sibling until caller resumed")
		}
	})
}

func TestCentralPages_FailureCancelsHigherWhileCallerIsBusy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := make(chan int, 2)
		cancelled := make(chan int, 2)
		release := make(chan struct{})
		transport := centralTransport(func(r *http.Request) (*http.Response, error) {
			var q struct {
				Page int `json:"pageNumber"`
			}
			if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
				return nil, err
			}
			if q.Page != 5 {
				started <- q.Page
				<-r.Context().Done()
				cancelled <- q.Page
				return nil, r.Context().Err()
			}
			<-release
			return &http.Response{StatusCode: 400, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		})
		client := vbpl.NewWithBase("https://example.test", "test", vbpl.WithTransport(transport), vbpl.WithLimits(float64(rate.Inf), 16))
		pages := newCentralPages(context.Background(), client)
		pages.start(2)
		pages.start(5)
		pages.start(6)
		<-started
		<-started
		close(release)
		synctest.Wait()
		higher := false
		lower := false
		for len(cancelled) > 0 {
			switch <-cancelled {
			case 2:
				lower = true
			case 6:
				higher = true
			}
		}
		_ = pages.close()
		if lower || !higher {
			t.Fatalf("lower cancelled=%v higher cancelled=%v", lower, higher)
		}
	})
}
