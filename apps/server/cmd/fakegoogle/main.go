package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"danny.vn/trachuan/apps/server/internal/weblogin/fakegoogle"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:28081", "loopback listen address")
	clientID := flag.String("client-id", "", "client ID")
	clientSecret := flag.String("client-secret", "", "client secret")
	redirectURI := flag.String("redirect-uri", "", "exact redirect URI")
	flag.Parse()
	if !fakegoogle.ValidListen(*listen) {
		fmt.Fprintln(os.Stderr, "invalid listen address")
		os.Exit(1)
	}
	handler, err := fakegoogle.New(fakegoogle.Config{Issuer: "http://" + *listen, ClientID: *clientID, ClientSecret: *clientSecret, RedirectURI: *redirectURI})
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid provider configuration")
		os.Exit(1)
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen failed")
		os.Exit(1)
	}
	fmt.Printf("fakegoogle listening on %s\n", listener.Addr())
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "provider failed")
		os.Exit(1)
	}
}
