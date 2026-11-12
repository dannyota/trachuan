package ocrvision

import (
	"context"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestToken_CallerCancellationStopsPreflightAndRefresh(t *testing.T) {
	for _, preflight := range []bool{true, false} {
		t.Run(map[bool]string{true: "preflight", false: "refresh"}[preflight], func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			source := tokenFunc(func() (*oauth2.Token, error) {
				close(started)
				<-release
				return &oauth2.Token{AccessToken: "test"}, nil
			})
			client := NewClient(Config{Project: "test", TokenSource: source, Wait: func(context.Context) error { return nil }})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				if preflight {
					done <- client.Preflight(ctx)
				} else {
					_, err := client.attempt(ctx, nil, newBudget(1))
					done <- err
				}
			}()
			<-started
			cancel()
			select {
			case err := <-done:
				if err == nil {
					t.Error("cancellation succeeded")
				}
				close(release)
			case <-time.After(100 * time.Millisecond):
				t.Error("token acquisition ignored caller cancellation")
				close(release)
				if err := <-done; err == nil {
					t.Error("token acquisition succeeded after cancellation")
				}
			}
		})
	}
}

func TestToken_ThirtySecondBoundIsInjectable(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	client := NewClient(Config{Project: "test", TokenSource: tokenFunc(func() (*oauth2.Token, error) { <-release; return &oauth2.Token{AccessToken: "test"}, nil }), TokenTimeout: 10 * time.Millisecond})
	done := make(chan error, 1)
	go func() { done <- client.Preflight(context.Background()) }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("timed-out token accepted")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("token acquisition ignored bound")
	}
	if tokenTimeout != 30*time.Second {
		t.Fatal("production bound changed")
	}
}
