package credential

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func response(v *Verifier, headers []string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	for _, header := range headers {
		r.Header.Add("Authorization", header)
	}
	w := httptest.NewRecorder()
	Middleware(v, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := FromContext(r.Context())
		if !ok || p.AccountID != 7 || p.Scope != "mcp:read" || p.Audience != v.Audience {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(w, r)
	return w
}

func TestMiddleware_HeadersAndStatus(t *testing.T) {
	for _, name := range []string{"missing", "valid", "mixed scheme", "wrong scheme", "duplicate wrong scheme", "bearer prefix", "double space", "tab", "trailing space", "newline", "duplicate", "combined", "unknown", "unavailable", "touch unavailable"} {
		t.Run(name, func(t *testing.T) {
			v, s, _, plain := fixture(t)
			headers := []string{"Bearer " + plain}
			status := http.StatusUnauthorized
			lookup := false
			switch name {
			case "missing":
				headers = nil
			case "valid":
				status, lookup = http.StatusNoContent, true
			case "mixed scheme":
				headers[0], status, lookup = "bEaReR "+plain, http.StatusNoContent, true
			case "wrong scheme":
				headers[0] = "Basic " + plain
			case "duplicate wrong scheme":
				headers = []string{"Basic " + plain, "Basic " + plain}
			case "bearer prefix":
				headers[0] = "BearerX " + plain
			case "double space":
				headers[0] = "Bearer  " + plain
			case "tab":
				headers[0] = "Bearer\t" + plain
			case "trailing space":
				headers[0] += " "
			case "newline":
				headers[0] += "\n"
			case "duplicate":
				headers = append(headers, headers[0])
			case "combined":
				headers[0] += ", " + headers[0]
			case "unknown":
				s.err, lookup = pgx.ErrNoRows, true
			case "unavailable":
				s.err, status, lookup = errors.New("sensitive failure"), http.StatusServiceUnavailable, true
			case "touch unavailable":
				s.touchErr, status, lookup = errors.New("sensitive failure"), http.StatusServiceUnavailable, true
			}
			w := response(v, headers)
			if w.Code != status {
				t.Fatalf("status=%d, want %d", w.Code, status)
			}
			challenge := ""
			if status == http.StatusUnauthorized {
				challenge = `Bearer resource_metadata="https://example.invalid/.well-known/oauth-protected-resource/mcp", scope="mcp:read"`
				if name != "missing" && name != "wrong scheme" && name != "duplicate wrong scheme" && name != "bearer prefix" {
					challenge += `, error="invalid_token"`
				}
			}
			if w.Header().Get("WWW-Authenticate") != challenge {
				t.Fatal("incorrect authentication challenge")
			}
			if (s.lookups > 0) != lookup {
				t.Fatal("unexpected store lookup")
			}
		})
	}
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("empty context has a principal")
	}
}

func TestMiddleware_CacheNeverOutlivesExpiryOrWindow(t *testing.T) {
	for _, name := range []string{"revoked", "blocked", "expiry", "default ttl", "short ttl", "oversized ttl", "unavailable"} {
		t.Run(name, func(t *testing.T) {
			v, s, now, plain := fixture(t)
			window := 10 * time.Second
			switch name {
			case "expiry":
				window = 3 * time.Second
				s.row.ExpiresAt = now.Add(window)
			case "default ttl":
				v.CacheTTL = 0
			case "short ttl":
				window = time.Second
				v.CacheTTL = window
			case "oversized ttl":
				v.CacheTTL = time.Hour
			}
			if w := response(v, []string{"Bearer " + plain}); w.Code != http.StatusNoContent {
				t.Fatal("valid token denied")
			}
			if name == "blocked" {
				s.row.BlockedAt = now
			} else if name == "unavailable" {
				s.err = errors.New("sensitive store failure")
			} else if name != "expiry" {
				s.row.RevokedAt = now
			}
			*now = now.Add(window - time.Nanosecond)
			if w := response(v, []string{"Bearer " + plain}); w.Code != http.StatusNoContent {
				t.Fatal("cache expired early")
			}
			*now = now.Add(time.Nanosecond)
			want := http.StatusUnauthorized
			if name == "unavailable" {
				want = http.StatusServiceUnavailable
			}
			if w := response(v, []string{"Bearer " + plain}); w.Code != want {
				t.Fatal("cached credential survived its deadline")
			}
			if s.lookups != 2 {
				t.Fatalf("lookups=%d, want 2", s.lookups)
			}
		})
	}
}

func TestMiddleware_NoTokenInLogs(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	_, hash, _ := Issue(bytes.NewReader(make([]byte, 32)))
	for _, phase := range []string{"valid", "unknown", "lookup failure", "touch failure"} {
		v, s, _, plain := fixture(t)
		failure := errors.New("query " + plain + " " + hash)
		switch phase {
		case "unknown":
			s.err = pgx.ErrNoRows
		case "lookup failure":
			s.err = failure
		case "touch failure":
			s.touchErr = failure
		}
		w := response(v, []string{"Bearer " + plain})
		for _, output := range []string{logs.String(), w.Body.String(), w.Header().Get("WWW-Authenticate")} {
			if strings.Contains(output, plain) || strings.Contains(output, hash) || strings.Contains(output, "query ") {
				t.Fatal("credential or database text disclosed")
			}
		}
	}
}
