package eventing

import (
	"context"
	"log/slog"
	"time"

	"github.com/FangcunMount/qs-server/internal/worker/handlers"
)

func decorateHandlerWithLogging(logger *slog.Logger, handlerName string, next handlers.HandlerFunc) handlers.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(ctx context.Context, eventType string, payload []byte) error {
		fields := handlerLogFields(handlerName, eventType, payload)
		logger.Info("worker handler started", fields...)

		startedAt := time.Now()
		err := next(ctx, eventType, payload)
		elapsedFields := append(fields, slog.Int64("elapsed_ms", time.Since(startedAt).Milliseconds()))
		if err != nil {
			logger.Error("worker handler failed",
				append(elapsedFields, slog.String("error", err.Error()))...,
			)
			return err
		}

		logger.Info("worker handler completed", elapsedFields...)
		return nil
	}
}

func handlerLogFields(handlerName, eventType string, payload []byte) []any {
	fields := []any{
		slog.String("handler", handlerName),
		slog.String("event_type", eventType),
		slog.Int("payload_bytes", len(payload)),
	}

	env, err := handlers.ParseEventEnvelope(payload)
	if err != nil {
		return append(fields, slog.String("envelope_parse_error", err.Error()))
	}

	fields = append(fields,
		slog.String("event_id", env.ID),
		slog.String("envelope_event_type", env.EventType),
		slog.String("aggregate_type", env.AggregateType),
		slog.String("aggregate_id", env.AggregateID),
	)
	if !env.OccurredAt.IsZero() {
		fields = append(fields, slog.Time("occurred_at", env.OccurredAt))
	}
	return fields
}
