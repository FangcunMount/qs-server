package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestCloseContextReportsTimeoutAndContinuesWaitingForOriginalHTTPHandler(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { close(entered); <-release }), ReadHeaderTimeout: time.Second}
	t.Cleanup(func() { once.Do(func() { close(release) }); _ = srv.Close(); _ = listener.Close() })
	go func() { _ = srv.Serve(listener) }()
	response := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}
		res, err := client.Get("http://" + listener.Addr().String())
		if res != nil {
			_ = res.Body.Close()
		}
		response <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("owned HTTP request absent")
	}
	host := &GenericAPIServer{insecureServer: srv}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := host.CloseContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("HTTP close hid its unfinished handler", err)
	}
	done := make(chan error, 1)
	go func() { done <- host.CloseContext(context.Background()) }()
	select {
	case err := <-done:
		t.Fatal("actual handler was not drained", err)
	case <-time.After(10 * time.Millisecond):
	}
	once.Do(func() { close(release) })
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP drain did not finish")
	}
	if err := <-response; err != nil {
		t.Fatal(err)
	}
}
