package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestMail_DisabledWarnsOnceWithoutStore(t *testing.T) {
	var logs bytes.Buffer
	stop, err := startMail(context.Background(), config{}, nil, &logs)
	if err != nil {
		t.Fatal(err)
	}
	stop()
	if strings.Count(logs.String(), "mail_disabled") != 1 || !strings.Contains(logs.String(), `"level":"WARN"`) {
		t.Fatal("missing disabled warning")
	}
}
