package source

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParse_PinsSurviveFreeze(t *testing.T) {
	raw := `country: vn
vbpl:
  selection_version: 1
  pinned:
    - key: vn:vbpl:pin
      number: 71/2099/QH15
      expand: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
      fetch: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
      rel: 20990101_1
      since: '2099-01-02'
`
	list, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`"pinned"`, `"vn:vbpl:pin"`, `"since":"2099-01-02"`, `"rel":"20990101_1"`} {
		if !strings.Contains(string(data), value) {
			t.Errorf("snapshot lost %s: %s", value, data)
		}
	}
}
