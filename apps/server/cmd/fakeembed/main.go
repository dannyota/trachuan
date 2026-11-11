package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"danny.vn/trachuan/apps/server/cmd/fakeembed/internal/fakeembed"
	"danny.vn/trachuan/apps/server/internal/weblogin/fakegoogle"
)

func main() {
	addr := flag.String("listen", "127.0.0.1:28082", "loopback listen address")
	flag.Parse()
	if !fakegoogle.ValidListen(*addr) {
		fmt.Fprintln(os.Stderr, "invalid listen address")
		os.Exit(1)
	}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen failed")
		os.Exit(1)
	}
	fmt.Printf("fakeembed listening on %s\n", listener.Addr())
	server := &http.Server{Handler: fakeembed.NewHandler(), ReadHeaderTimeout: 5 * time.Second}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "embedder failed")
		os.Exit(1)
	}
}
