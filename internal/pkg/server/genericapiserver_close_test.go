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

func TestGenericAPIServerCloseShutsDownBothListenersWithinSharedBudget(t *testing.T) {
	type blockingServer struct {
		server      *httptest.Server
		entered     chan struct{}
		release     chan struct{}
		requestDone chan struct{}
	}
	start := func() blockingServer {
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
		return blockingServer{server: testServer, entered: entered, release: release, requestDone: requestDone}
	}

	secure := start()
	insecure := start()
	server := &GenericAPIServer{
		secureServer:    secure.server.Config,
		insecureServer:  insecure.server.Config,
		ShutdownTimeout: 30 * time.Millisecond,
	}

	startedAt := time.Now()
	server.Close()
	if elapsed := time.Since(startedAt); elapsed > 500*time.Millisecond {
		t.Fatalf("Close() exceeded shared shutdown budget: %s", elapsed)
	}

	close(secure.release)
	close(insecure.release)
	<-secure.requestDone
	<-insecure.requestDone
	secure.server.Close()
	insecure.server.Close()
}
