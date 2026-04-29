package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricsServer struct {
	server   *http.Server
	listener net.Listener
}

func NewMetricsServerWithGovernance(bindAddress string, bindPort int, component string, registry *observability.FamilyStatusRegistry) *MetricsServer {
	return NewMetricsServerWithGovernanceAndResilience(bindAddress, bindPort, component, registry, nil)
}

func NewMetricsServerWithGovernanceAndResilience(
	bindAddress string,
	bindPort int,
	component string,
	registry *observability.FamilyStatusRegistry,
	resilience func() resilienceplane.RuntimeSnapshot,
) *MetricsServer {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeGovernanceJSON(w, http.StatusOK, map[string]interface{}{
			"status":    "healthy",
			"component": component,
			"redis":     observability.SnapshotForComponent(component, registry),
		})
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		snapshot := observability.SnapshotForComponent(component, registry)
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
		writeGovernanceJSON(w, http.StatusOK, observability.SnapshotForComponent(component, registry))
	})
	mux.HandleFunc("/governance/resilience", func(w http.ResponseWriter, _ *http.Request) {
		if resilience == nil {
			writeGovernanceJSON(w, http.StatusOK, resilienceplane.FinalizeRuntimeSnapshot(resilienceplane.NewRuntimeSnapshot(component, time.Now())))
			return
		}
		writeGovernanceJSON(w, http.StatusOK, resilience())
	})

	return &MetricsServer{
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

func (s *MetricsServer) Start() error {
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

func (s *MetricsServer) Shutdown(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}
