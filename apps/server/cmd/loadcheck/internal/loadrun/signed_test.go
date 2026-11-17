package loadrun

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestSigned_PhaseDNeverAborts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer server.Close()
	f, err := os.CreateTemp(t.TempDir(), "requests")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	q, err := newQueries([]string{"fixture"})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &recorder{file: f}
	run := runner{config: Config{Pages: server.URL, API: server.URL, PublicURL: server.URL}, accounts: []account{{client: server.Client()}}, keys: []string{"vn:fixture:1"}, queries: q, output: recorder, abortSeed: 42}
	i := 0
	for !abortRequest(i, 42) {
		i++
	}
	run.search(context.Background(), "D", i)
	run.reader(context.Background(), "D", i)
	for _, record := range recorder.records {
		if record.Aborted || record.Status != 200 {
			t.Fatalf("signed request %+v", record)
		}
	}
}
