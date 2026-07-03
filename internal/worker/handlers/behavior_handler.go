package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
)

func handleBehaviorProjector(deps *Dependencies) HandlerFunc {
	logger := behaviorProjectorLogger(deps)
	return func(ctx context.Context, eventType string, payload []byte) error {
		if deps == nil || deps.InternalClient == nil {
			logger.Error("behavior projector internal client is not available",
				slog.String("event_type", eventType),
				slog.Int("payload_bytes", len(payload)),
			)
			return fmt.Errorf("internal client is not available")
		}

		mapper, ok := behaviorEventMappers[eventType]
		if !ok {
			return fmt.Errorf("unsupported behavior event type %q", eventType)
		}
		req, err := mapper(payload)
		if err != nil {
			return err
		}
		return projectBehaviorEvent(ctx, logger, deps, req)
	}
}

func projectBehaviorEvent(ctx context.Context, logger *slog.Logger, deps *Dependencies, req *pb.ProjectBehaviorEventRequest) error {
	if logger == nil {
		logger = slog.Default()
	}
	fields := behaviorProjectLogFields(req)
	logger.Info("projecting behavior event", fields...)
	startedAt := time.Now()

	resp, err := deps.InternalClient.ProjectBehaviorEvent(ctx, req)
	if err != nil {
		logger.Error("behavior projection grpc failed",
			append(fields,
				slog.Int64("elapsed_ms", time.Since(startedAt).Milliseconds()),
				slog.String("error", err.Error()),
			)...,
		)
		return err
	}
	if resp == nil {
		logger.Error("behavior projection returned empty response",
			append(fields,
				slog.Int64("elapsed_ms", time.Since(startedAt).Milliseconds()),
			)...,
		)
		return fmt.Errorf("behavior projector returned empty response")
	}

	logger.Info("behavior projection response received",
		append(fields,
			slog.String("projector_status", resp.GetStatus()),
			slog.String("projector_message", resp.GetMessage()),
			slog.Int64("elapsed_ms", time.Since(startedAt).Milliseconds()),
		)...,
	)
	return nil
}

func behaviorProjectorLogger(deps *Dependencies) *slog.Logger {
	if deps != nil && deps.Logger != nil {
		return deps.Logger
	}
	return slog.Default()
}

func behaviorProjectLogFields(req *pb.ProjectBehaviorEventRequest) []any {
	if req == nil {
		return []any{slog.Bool("request_nil", true)}
	}

	fields := []any{
		slog.String("event_id", req.GetEventId()),
		slog.String("event_type", req.GetEventType()),
		slog.Int64("org_id", req.GetOrgId()),
		slog.Uint64("clinician_id", req.GetClinicianId()),
		slog.Uint64("source_clinician_id", req.GetSourceClinicianId()),
		slog.Uint64("entry_id", req.GetEntryId()),
		slog.Uint64("testee_id", req.GetTesteeId()),
		slog.Uint64("answersheet_id", req.GetAnswersheetId()),
		slog.Uint64("assessment_id", req.GetAssessmentId()),
		slog.Uint64("report_id", req.GetReportId()),
	}
	if req.GetFailureReason() != "" {
		fields = append(fields, slog.String("failure_reason", req.GetFailureReason()))
	}
	if occurredAt := req.GetOccurredAt(); occurredAt != nil {
		fields = append(fields, slog.Time("occurred_at", occurredAt.AsTime()))
	}
	return fields
}
