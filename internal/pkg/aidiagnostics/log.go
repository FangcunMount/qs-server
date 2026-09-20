// Package aidiagnostics emits only fixed AI transport metadata, never payloads.
package aidiagnostics

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var output = slog.New(slog.NewJSONHandler(os.Stdout, nil))

func Completed(ctx context.Context, method string, started time.Time, err error, requestID, eventID string) {
	md, _ := metadata.FromIncomingContext(ctx)
	if outgoing, ok := metadata.FromOutgoingContext(ctx); ok {
		md = outgoing
	}
	correlation := ""
	if values := md.Get("x-correlation-id"); len(values) == 1 {
		if id, e := uuid.Parse(values[0]); e == nil {
			correlation = id.String()
		}
	}
	fields := []any{"schema_version", 1, "service", "qs-apiserver", "component", "ai_bridge", "event", "rpc.completed", "log_event_id", uuid.NewString(), "correlation_id", correlation, "stage", method, "status", status.Code(err).String(), "duration_ms", float64(time.Since(started).Microseconds()) / 1000}
	for key, value := range map[string]string{"request_id": requestID, "delivery_event_id": eventID} {
		if id, e := uuid.Parse(value); e == nil {
			fields = append(fields, key, id.String())
		}
	}
	output.Info("", fields...)
}
