package accountemail

import (
	"encoding/json"
	"os"
	"testing"
)

func TestParse_Rules(t *testing.T) {
	data, err := os.ReadFile("testdata/addresses.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Name, Raw, Want string }
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			got, err := Parse(tc.Raw)
			if tc.Want == "" {
				if err == nil {
					t.Fatal("accepted invalid address")
				}
				return
			}
			if err != nil || got != tc.Want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.Want)
			}
		})
	}
}
