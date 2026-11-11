package weblogin

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/contracttest"
)

func testLoginContract(t *testing.T, coverage *contracttest.Coverage) {
	f := newFixture(t)
	w := httptest.NewRecorder()
	f.h.Start.ServeHTTP(w, httptest.NewRequest("GET", startPath, nil))
	if w.Code != 302 {
		t.Fatalf("status %d", w.Code)
	}
	coverage.Check(t, "GET", "/api/v1/auth/google/start", w)
	w = httptest.NewRecorder()
	f.h.Callback.ServeHTTP(w, httptest.NewRequest("GET", callbackPath, nil))
	if w.Code != 302 {
		t.Fatalf("status %d", w.Code)
	}
	coverage.Check(t, "GET", "/api/v1/auth/google/callback", w)
}

func testLoginContract_Unavailable(t *testing.T, coverage *contracttest.Coverage) {
	f := newFixture(t)
	f.store.createErr = errors.New("fixture")
	w := httptest.NewRecorder()
	f.h.Start.ServeHTTP(w, httptest.NewRequest("GET", startPath, nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	coverage.Check(t, "GET", startPath, w)
	f = newFixture(t)
	a := f.begin(t, "/")
	f.store.claimErr = errors.New("fixture")
	w = f.callback(a, "code")
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	coverage.Check(t, "GET", callbackPath, w)
}

func TestOpenAPIContract(t *testing.T) {
	c, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for path := range c.Paths {
		if strings.HasPrefix(path, "/api/v1/auth/google/") {
			paths = append(paths, path)
		}
	}
	coverage := contracttest.NewCoverage(c, paths)
	t.Run("TestLoginContract", func(t *testing.T) { testLoginContract(t, coverage) })
	t.Run("TestLoginContract_Unavailable", func(t *testing.T) { testLoginContract_Unavailable(t, coverage) })
	if missing := coverage.Missing(); len(missing) > 0 {
		t.Errorf("unexercised responses: %v", missing)
	}
}
