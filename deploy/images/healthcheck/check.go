package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

func check(ctx context.Context, target string) error {
	u, err := url.Parse(target)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
		return errors.New("invalid probe target")
	}
	switch u.Scheme {
	case "tcp":
		if u.Path != "" || u.Port() == "" {
			return errors.New("invalid TCP target")
		}
		var dialer net.Dialer
		connection, err := dialer.DialContext(ctx, "tcp", u.Host)
		if err != nil {
			return fmt.Errorf("connect probe: %w", err)
		}
		if err := connection.Close(); err != nil {
			return fmt.Errorf("close probe: %w", err)
		}
		return nil
	case "http":
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return fmt.Errorf("create probe: %w", err)
		}
		client := &http.Client{Transport: &http.Transport{}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
		defer client.CloseIdleConnections()
		response, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("request probe: %w", err)
		}
		if err := response.Body.Close(); err != nil {
			return fmt.Errorf("close probe body: %w", err)
		}
		if response.StatusCode != http.StatusOK {
			return errors.New("unhealthy HTTP status")
		}
		return nil
	default:
		return errors.New("unsupported probe scheme")
	}
}

func run(args []string) int {
	if len(args) != 1 {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if check(ctx, args[0]) != nil {
		return 1
	}
	return 0
}
