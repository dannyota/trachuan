package release

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/release/waitlog"
)

func TestReaderWait_Minute(t *testing.T) {
	var output bytes.Buffer
	tracer := newReaderWait(slog.New(slog.NewJSONHandler(&output, nil)), "fixture")
	tracer.flush()
	if output.Len() != 0 {
		t.Fatal("quiet minute logged")
	}
	tracer.record(time.Millisecond)
	tracer.record(12 * time.Millisecond)
	tracer.record(60 * time.Millisecond)
	tracer.flush()
	line := output.String()
	for _, want := range []string{`"msg":"reader pool wait"`, `"count":3`, `"p50_ms":16`, `"p99_ms":64`, `"max_ms":60`, `"bucket_bounds_ms":[1,2,4,8,16,32,50,64,128,256,512,1024]`, `"bucket_counts":[1,0,0,0,1,0,0,1,0,0,0,0,0]`} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %s in %s", want, line)
		}
	}
	output.Reset()
	tracer.flush()
	if output.Len() != 0 {
		t.Fatal("second quiet minute logged")
	}
}

func TestReaderWait_Threshold(t *testing.T) {
	var output bytes.Buffer
	tracer := newReaderWait(slog.New(slog.NewJSONHandler(&output, nil)), "fixture")
	tracer.record(49 * time.Millisecond)
	tracer.flush()
	if !strings.Contains(output.String(), `"p99_ms":50`) {
		t.Fatal(output.String())
	}
}

func TestReaderWait_NilLogger(t *testing.T) {
	tracer := newReaderWait(nil, "fixture")
	tracer.record(time.Millisecond)
	tracer.flush()
}

func TestReaderWait_ResolverLoggerAndReportParser(t *testing.T) {
	var output bytes.Buffer
	resolver := Resolver{Logger: slog.New(slog.NewJSONHandler(&output, nil))}
	tracer := resolver.readerWait("fixture")
	tracer.record(49 * time.Millisecond)
	tracer.flush()
	var line map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &line); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, name := range []string{"msg", "time", "count", "p50_ms", "p99_ms", "max_ms", "bucket_bounds_ms", "bucket_counts"} {
		if _, ok := line[name]; !ok {
			t.Errorf("missing %s", name)
		}
	}
	now := time.Now()
	value, err := waitlog.P99(bytes.NewReader(output.Bytes()), now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil || value != 50 {
		t.Fatalf("report p99 = %v, error = %v", value, err)
	}
}

func TestReaderWait_RetirementFlushes(t *testing.T) {
	var output bytes.Buffer
	tracer := newReaderWait(slog.New(slog.NewJSONHandler(&output, nil)), "fixture")
	tracer.record(49 * time.Millisecond)
	stop := make(chan struct{})
	close(stop)
	tracer.run(stop)
	if !strings.Contains(output.String(), `"count":1`) {
		t.Fatal("retirement lost pending wait")
	}
}

type blockedWaitWriter struct {
	entered chan struct{}
	release chan struct{}
}

func (w *blockedWaitWriter) Write(p []byte) (int, error) {
	close(w.entered)
	<-w.release
	return len(p), nil
}
func TestReaderWait_LoggingDoesNotBlockRecord(t *testing.T) {
	writer := &blockedWaitWriter{entered: make(chan struct{}), release: make(chan struct{})}
	tracer := newReaderWait(slog.New(slog.NewJSONHandler(writer, nil)), "fixture")
	tracer.record(time.Millisecond)
	flushed := make(chan struct{})
	go func() { tracer.flush(); close(flushed) }()
	<-writer.entered
	recorded := make(chan struct{})
	go func() { tracer.record(time.Millisecond); close(recorded) }()
	select {
	case <-recorded:
	case <-time.After(25 * time.Millisecond):
		close(writer.release)
		<-flushed
		<-recorded
		t.Fatal("record blocked by logging")
	}
	close(writer.release)
	<-flushed
}
