package aibridge

import (
	"context"
	"encoding/json"
	"github.com/FangcunMount/qs-server/internal/pkg/aidiagnostics"

	"os"
	"regexp"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const correlationMetadata = "x-correlation-id"

var correlationPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// correlateRPC only adds diagnostic metadata. It never changes authorization or retries.
func correlateRPC(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	id := NormalizeCorrelationID(middleware.RequestIDFromStandardContext(ctx))
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set(correlationMetadata, id)
	ctx = metadata.NewOutgoingContext(ctx, md)
	started := time.Now()
	err := invoke(ctx, method, req, reply, cc, opts...)
	requestID := ""
	if request, ok := req.(interface{ GetRequestId() string }); ok {
		requestID = request.GetRequestId()
	}
	aidiagnostics.Completed(ctx, method, started, err, requestID, "")
	return err
}

// NormalizeCorrelationID accepts a validated id or mints a UUID.
func NormalizeCorrelationID(value string) string {
	if correlationPattern.MatchString(value) {
		return value
	}
	return uuid.NewString()
}

// CorrelationFromIncoming reads x-correlation-id (or legacy x-request-id) from metadata.
func CorrelationFromIncoming(ctx context.Context) string {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get(correlationMetadata); len(values) > 0 {
			return NormalizeCorrelationID(values[0])
		}
		if values := md.Get("x-request-id"); len(values) > 0 {
			return NormalizeCorrelationID(values[0])
		}
	}
	return uuid.NewString()
}

// EmitBoundary writes one schema_version=1 JSON line for AI boundary ops. Never logs payloads.
func EmitBoundary(event, status, errorClass, correlationID, requestID, deliveryEventID string) {
	record := map[string]any{
		"schema_version": 1,
		"timestamp":      time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		"level":          "INFO",
		"service":        "qs-apiserver",
		"environment":    getenv("QS_ENVIRONMENT", "production"),
		"event":          event,
		"component":      "aibridge.results",
		"log_event_id":   uuid.NewString(),
		"correlation_id": NormalizeCorrelationID(correlationID),
		"status":         status,
	}
	if id, err := uuid.Parse(requestID); err == nil {
		record["request_id"] = id.String()
	}
	if id, err := uuid.Parse(deliveryEventID); err == nil {
		record["delivery_event_id"] = id.String()
	}
	if errorClass != "" {
		record["error_class"] = errorClass
		record["level"] = "WARNING"
	}
	line, err := json.Marshal(record)
	if err != nil {
		return
	}
	_, _ = os.Stdout.Write(append(line, '\n'))
}

func getenv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
