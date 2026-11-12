package ocrvision

import "net/http"

func directHTTPClient(base *http.Client) *http.Client {
	if base == nil {
		base = &http.Client{}
	}
	client := *base
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	if concrete, ok := transport.(*http.Transport); ok {
		isolated := concrete.Clone()
		isolated.Proxy = nil
		transport = isolated
	}
	client.Transport = transport
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &client
}
