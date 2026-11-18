package observe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConnections_ReadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.txt")
	if err := os.WriteFile(path, []byte("2026-01-01T00:00:00Z 3\n2026-01-01T00:00:01Z 41\n"), 0600); err != nil {
		t.Fatal(err)
	}
	points, err := readConnections(path)
	if err != nil || len(points) != 2 || points[1].Connections == nil || *points[1].Connections != 41 {
		t.Fatalf("%+v %v", points, err)
	}
}

func TestConnections_MalformedLineIsError(t *testing.T) {
	for _, line := range []string{"2026-01-01T00:00:00Z\n", "2026-01-01T00:00:00Z x\n", "yesterday 3\n", "2026-01-01T00:00:00Z -1\n", "2026-01-01T00:00:00+07:00 3\n", "\n", "2026-01-01T00:00:00Z 3 4\n"} {
		path := filepath.Join(t.TempDir(), "connections.txt")
		if err := os.WriteFile(path, []byte("2026-01-01T00:00:00Z 3\n"+line), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readConnections(path); err == nil {
			t.Errorf("accepted %q", line)
		}
	}
}

func TestConnections_PartialLastLineIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.txt")
	if err := os.WriteFile(path, []byte("2026-01-01T00:00:00Z 3\n2026-01-01T00:00:01Z 4"), 0600); err != nil {
		t.Fatal(err)
	}
	points, err := readConnections(path)
	if err != nil || len(points) != 1 || *points[0].Connections != 3 {
		t.Fatalf("%+v %v", points, err)
	}
	if err = os.WriteFile(path, []byte("2026-01-01T00:00:00Z 3\n2026-01-01T00:00:0"), 0600); err != nil {
		t.Fatal(err)
	}
	if points, err = readConnections(path); err != nil || len(points) != 1 {
		t.Fatalf("%+v %v", points, err)
	}
	if err = os.WriteFile(path, []byte("2026-01-01T00:00:00Z 3\ngarbage\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = readConnections(path); err == nil {
		t.Fatal("complete malformed last line accepted")
	}
}
