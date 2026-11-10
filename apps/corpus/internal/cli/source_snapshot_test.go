package cli

import "testing"

func TestSourceSnapshot_PreviousEncoding(t *testing.T) {
	data := []byte("country: vn\nvbpl:\n  sweep_agencies: ['62']\n")
	got, err := sourceSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"country":"vn","vbpl":{"sweep_agencies":["62"]}}`
	if string(got) != want {
		t.Fatalf("snapshot = %s, want %s", got, want)
	}
}
