package authmail

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplates_AllKinds(t *testing.T) {
	for _, kind := range []string{"registration", "account_exists", "add_password", "reset", "password_changed", "totp_enrolled", "totp_removed", "recovery_codes_replaced"} {
		msg, err := Render("https://example.test", kind, Payload{To: "person@example.test", Token: "synthetic-token"})
		if err != nil {
			t.Fatal(err)
		}
		if msg.To != "person@example.test" || !strings.Contains(msg.Subject, " / ") || !strings.Contains(msg.Text, "English\n") {
			t.Fatal("missing bilingual message")
		}
		link := strings.Contains(msg.Text, "https://example.test/link#synthetic-token")
		if link != (kind == "registration" || kind == "reset" || kind == "add_password") {
			t.Fatal("wrong link template")
		}
	}
	if _, err := Render("https://example.test", "unknown", Payload{To: "person@example.test"}); err == nil {
		t.Fatal("unknown kind accepted")
	}
	if _, err := Render("https://example.test", "reset", Payload{To: "person@example.test"}); err == nil {
		t.Fatal("missing token accepted")
	}
}

func TestCapture_PrivateMessage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "capture")
	sender, err := NewCapture(dir)
	if err != nil {
		t.Fatal(err)
	}
	msg := Message{To: "person@example.test", Subject: "Synthetic", Text: "Synthetic message"}
	if err = sender.Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatal("no captured file")
	}
	path := filepath.Join(dir, files[0].Name())
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("unsafe file mode")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got Message
	if err = json.Unmarshal(data, &got); err != nil || got != msg {
		t.Fatal("message differs")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = sender.Send(ctx, msg); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestTemplates_RejectsHeaderInjection(t *testing.T) {
	for _, kind := range []string{"registration", "account_exists", "reset", "add_password", "password_changed"} {
		for _, field := range []string{"recipient", "token", "url", "kind"} {
			t.Run(kind+"/"+field, func(t *testing.T) {
				payload := Payload{To: "person@example.test", Token: "synthetic-token"}
				publicURL, templateKind := "https://example.test", kind
				switch field {
				case "recipient":
					payload.To += "\r\nBcc: attacker@example.test"
				case "token":
					payload.Token += "\r\nBcc: attacker@example.test"
				case "url":
					publicURL += "\r\nBcc: attacker@example.test"
				case "kind":
					templateKind += "\r\nBcc: attacker@example.test"
				}
				if _, err := Render(publicURL, templateKind, payload); err == nil {
					t.Fatal("accepted CR/LF in template field")
				}
			})
		}
	}
}
