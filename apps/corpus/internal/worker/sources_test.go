package worker

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/config"
)

func TestSources_RejectsNonLoopbackTestOrigins(t *testing.T) {
	_, _, err := sourceClients(config.Config{ChinhphuURL: "http://example.com", ChinhphuFileURL: "http://example.com", ChinhphuLoopbackHTTP: true}, nil)
	if err == nil {
		t.Fatal("accepted non-loopback origin")
	}
}
