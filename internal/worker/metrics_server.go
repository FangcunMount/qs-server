package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type metricsServer struct {
	server   *http.Server
	listener net.Listener
}

func newMetricsServerWithGovernance(bindAddress string, bindPort int, component string, registry *cacheobservability.FamilyStatusRegistry) *metricsServer {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeGovernanceJSON(w, http.StatusOK, map[string]interface{}{
			"status":    "healthy",
			"component": component,
			"redis":     cacheobservability.SnapshotForComponent(component, registry),
		})
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		snapshot := cacheobservability.SnapshotForComponent(component, registry)
		statusCode := http.StatusOK
		statusText := "ready"
		if !snapshot.Summary.Ready {
			statusCode = http.StatusServiceUnavailable
			statusText = "degraded"
		}
		writeGovernanceJSON(w, statusCode, map[string]interface{}{
			"status":    statusText,
			"component": component,
			"redis":     snapshot,
		})
	})
	mux.HandleFunc("/governance/redis", func(w http.ResponseWriter, _ *http.Request) {
		writeGovernanceJSON(w, http.StatusOK, cacheobservability.SnapshotForComponent(component, registry))
	})

	return &metricsServer{
		server: &http.Server{
			Addr:              net.JoinHostPort(bindAddress, fmt.Sprintf("%d", bindPort)),
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

func writeGovernanceJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
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
