package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheck_HTTPStatus(t *testing.T) {
	for _, status := range []int{200, 302, 404, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
			defer s.Close()
			err := check(context.Background(), s.URL)
			if (err == nil) != (status == 200) {
				t.Fatalf("check returned %v for status %d", err, status)
			}
		})
	}
}

func TestCheck_TCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := "tcp://" + listener.Addr().String()
	if err := check(context.Background(), address); err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := check(context.Background(), address); err == nil {
		t.Fatal("closed port accepted")
	}
}

func TestCheck_InvalidTarget(t *testing.T) {
	for _, target := range []string{"", "ftp://127.0.0.1", "tcp://127.0.0.1", "http://user:password@localhost", "http://localhost/#fragment"} {
		if check(context.Background(), target) == nil {
			t.Errorf("accepted invalid target %q", target)
		}
	}
}
