// Package aidiagnostics emits only fixed AI transport metadata, never payloads.
package aidiagnostics

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/FangcunMount/qs-server/pkg/version"
	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var correlationPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
var instance = uuid.NewString()
var queue = make(chan []byte, 1024)
var dropped atomic.Uint64

func init() {
	go func() {
		for line := range queue {
			_, _ = os.Stdout.Write(line)
		}
	}()
}

// Diagnostic failure cannot block a receipt transaction or RPC response.
func emit(fields map[string]any) {
	fields["schema_version"] = 1
	fields["timestamp"] = time.Now().UTC().Format(time.RFC3339Nano)
	fields["service"] = "qs-apiserver"
	fields["environment"] = os.Getenv("QS_ENVIRONMENT")
	if fields["environment"] == "" {
		fields["environment"] = "unknown"
	}
	fields["release"] = version.Get().GitCommit
	fields["instance_id"] = instance
	fields["log_event_id"] = uuid.NewString()
	line, err := json.Marshal(fields)
	if err != nil {
		return
	}
	select {
	case queue <- append(line, '\n'):
	default:
		dropped.Add(1)
	}
}

func NormalizeCorrelationID(value string) string {
	if correlationPattern.MatchString(value) {
		return value
	}
	return uuid.NewString()
}

func IncomingCorrelation(ctx context.Context) string {
	md, _ := metadata.FromIncomingContext(ctx)
	for _, name := range []string{"x-correlation-id", "x-request-id"} {
		if values := md.Get(name); len(values) == 1 {
			return NormalizeCorrelationID(values[0])
		}
	}
	return uuid.NewString()
}

func Completed(ctx context.Context, method string, started time.Time, err error, requestID, eventID string) {
	md, _ := metadata.FromOutgoingContext(ctx)
	correlation := ""
	if values := md.Get("x-correlation-id"); len(values) == 1 {
		correlation = NormalizeCorrelationID(values[0])
	}
	fields := record("rpc.completed", status.Code(err).String(), "", correlation, requestID, eventID)
	if err != nil {
		fields["level"] = "WARNING"
	}
	fields["stage"] = method
	fields["duration_ms"] = float64(time.Since(started).Microseconds()) / 1000
	emit(fields)
}

func record(event, state, errorClass, correlationID, requestID, eventID string) map[string]any {
	fields := map[string]any{"component": "ai_bridge", "event": event, "status": state, "level": "INFO", "correlation_id": NormalizeCorrelationID(correlationID)}
	for key, value := range map[string]string{"request_id": requestID, "delivery_event_id": eventID} {
		if id, err := uuid.Parse(value); err == nil {
			fields[key] = id.String()
		}
	}
	if errorClass != "" {
		fields["error_code"] = errorClass
		fields["level"] = "WARNING"
	}
	return fields
}

func Boundary(event, state, errorClass, correlationID, requestID, eventID string) {
	emit(record(event, state, errorClass, correlationID, requestID, eventID))
}
