package ocrvision

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/sys/unix"
)

// NewAuthenticated loads only the named credential file without exposing values.
func NewAuthenticated(ctx context.Context) (*Client, error) {
	return newAuthenticated(ctx, directHTTPClient(nil), tokenTimeout)
}

func newAuthenticated(ctx context.Context, httpClient *http.Client, timeout time.Duration) (*Client, error) {
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if project == "" {
		return nil, credentialError()
	}
	data, err := readCredentials(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
	if err != nil {
		return nil, err
	}
	// JWT token sources use this client even when they omit request contexts.
	tokenClient := *directHTTPClient(httpClient)
	if timeout <= 0 || timeout > tokenTimeout {
		timeout = tokenTimeout
	}
	tokenClient.Timeout = timeout
	authContext := context.WithValue(ctx, oauth2.HTTPClient, &tokenClient)
	//nolint:staticcheck // The operator controls this local credential file and its ADC type.
	credentials, err := google.CredentialsFromJSON(authContext, data, "https://www.googleapis.com/auth/cloud-vision")
	if err != nil {
		return nil, credentialError()
	}
	client := NewClient(Config{Project: project, TokenSource: credentials.TokenSource, HTTP: httpClient, TokenTimeout: timeout})
	if err = client.Preflight(ctx); err != nil {
		return nil, err
	}
	return client, nil
}

func credentialError() error {
	return fmt.Errorf("GOOGLE_APPLICATION_CREDENTIALS must name a readable regular credential file; GOOGLE_CLOUD_PROJECT is required")
}

func readCredentials(path string) ([]byte, error) {
	if path == "" {
		return nil, credentialError()
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, credentialError()
	}
	file := os.NewFile(uintptr(fd), "Vision credentials")
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0444 == 0 {
		return nil, credentialError()
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, credentialError()
	}
	return data, nil
}
