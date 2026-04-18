package worker

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type metricsServer struct {
	server   *http.Server
	listener net.Listener
}

func newMetricsServer(bindAddress string, bindPort int) *metricsServer {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return &metricsServer{
		server: &http.Server{
			Addr:              net.JoinHostPort(bindAddress, fmt.Sprintf("%d", bindPort)),
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

func (s *metricsServer) Start() error {
	if s == nil || s.server == nil {
		return nil
	}

	listener, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return err
	}
	s.listener = listener

	go func() {
		log.Infof("Starting worker metrics server on %s", s.server.Addr)
		if serveErr := s.server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			log.Errorf("worker metrics server stopped unexpectedly: %v", serveErr)
		}
	}()

	return nil
}

func (s *metricsServer) Shutdown(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}
