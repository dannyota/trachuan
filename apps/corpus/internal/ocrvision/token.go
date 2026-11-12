package ocrvision

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2"
)

const tokenTimeout = 30 * time.Second

func (c *Client) token(ctx context.Context) (*oauth2.Token, error) {
	timeout := c.tokenTimeout
	if timeout <= 0 || timeout > tokenTimeout {
		timeout = tokenTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("token acquisition canceled")
	}
	type result struct {
		token *oauth2.Token
		err   error
	}
	done := make(chan result, 1)
	// Some ADC sources post without the caller's context. Their HTTP client has
	// its own timeout; the buffered result lets a late response be discarded.
	go func() { token, err := c.tokens.Token(); done <- result{token, err} }()
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("token acquisition canceled")
	case r := <-done:
		if ctx.Err() != nil || r.err != nil || r.token == nil || !r.token.Valid() {
			return nil, fmt.Errorf("token acquisition failed")
		}
		return r.token, nil
	}
}
