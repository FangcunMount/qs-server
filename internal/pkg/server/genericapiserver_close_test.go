package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGenericAPIServerCloseAllowsDisabledListeners(t *testing.T) {
	t.Parallel()
	(&GenericAPIServer{}).Close()
}

func TestGenericAPIServerCloseAllowsOnlyInsecureListener(t *testing.T) {
	t.Parallel()
	s := &GenericAPIServer{insecureServer: &http.Server{}}
	s.Close()
}

func TestGenericAPIServerCloseAllowsOnlySecureListener(t *testing.T) {
	t.Parallel()
	s := &GenericAPIServer{secureServer: &http.Server{}}
	s.Close()
}

func TestGenericAPIServerCloseRespectsSharedShutdownTimeout(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	testServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release
	}))
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		response, err := http.Get(testServer.URL) //nolint:gosec // local test server
		if err == nil {
			_ = response.Body.Close()
		}
	}()
	<-entered

	server := &GenericAPIServer{
		insecureServer:  testServer.Config,
		ShutdownTimeout: 20 * time.Millisecond,
	}
	startedAt := time.Now()
	server.Close()
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("Close() exceeded shared shutdown budget: %s", elapsed)
	}

	close(release)
	<-requestDone
	testServer.Close()
}
