package main

import (
	"errors"
	"net"
	"os"
	"testing"
)

func TestServer_PortFromEnv(t *testing.T) {
	const wantPort = "19876"

	var gotAddr string
	listenFn := func(network, addr string) (net.Listener, error) {
		gotAddr = addr
		lis, err := net.Listen(network, addr)
		if err != nil {
			t.Skipf("could not bind to %s: %v", addr, err)
		}
		lis.Close()
		return lis, nil
	}

	environ := func(key string) string {
		if key == "DNS_MANAGER_PORT" {
			return wantPort
		}
		return ""
	}

	run(nil, environ, listenFn, os.Stderr)

	wantAddr := ":" + wantPort
	if gotAddr != wantAddr {
		t.Errorf("listener addr = %q, want %q", gotAddr, wantAddr)
	}
}

func TestServer_FailToListen(t *testing.T) {
	listenFn := func(network, addr string) (net.Listener, error) {
		return nil, errors.New("bind: address already in use")
	}

	environ := func(key string) string { return "" }

	err := run(nil, environ, listenFn, os.Stderr)
	if err == nil {
		t.Fatal("run() expected error, got nil")
	}
}
