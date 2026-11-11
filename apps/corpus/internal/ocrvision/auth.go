package ocrvision

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/oauth2/google"
)

// NewAuthenticated loads ADC without exposing credentials or provider errors.
func NewAuthenticated(ctx context.Context) (*Client, error) {
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if project == "" {
		return nil, fmt.Errorf("GOOGLE_CLOUD_PROJECT and GOOGLE_APPLICATION_CREDENTIALS are required")
	}
	credentials, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-vision")
	if err != nil {
		return nil, fmt.Errorf("GOOGLE_APPLICATION_CREDENTIALS must provide usable credentials; GOOGLE_CLOUD_PROJECT is required")
	}
	client := NewClient(Config{Project: project, TokenSource: credentials.TokenSource})
	if err = client.Preflight(ctx); err != nil {
		return nil, err
	}
	return client, nil
}
