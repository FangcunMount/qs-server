package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
	"google.golang.org/protobuf/types/known/timestamppb"
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

		switch eventType {
		case domainStatistics.EventTypeFootprintEntryOpened:
			var data domainStatistics.FootprintEntryOpenedData
			env, err := ParseEventData(payload, &data)
			if err != nil {
				return fmt.Errorf("failed to parse footprint entry opened event: %w", err)
			}
			return projectBehaviorEvent(ctx, logger, deps, &pb.ProjectBehaviorEventRequest{
				EventId:     env.ID,
				EventType:   eventType,
				OrgId:       data.OrgID,
				ClinicianId: data.ClinicianID,
				EntryId:     data.EntryID,
				OccurredAt:  timestamppb.New(data.OccurredAt),
			})
		case domainStatistics.EventTypeFootprintIntakeConfirmed:
			var data domainStatistics.FootprintIntakeConfirmedData
			env, err := ParseEventData(payload, &data)
			if err != nil {
				return fmt.Errorf("failed to parse footprint intake confirmed event: %w", err)
			}
			return projectBehaviorEvent(ctx, logger, deps, &pb.ProjectBehaviorEventRequest{
				EventId:     env.ID,
				EventType:   eventType,
				OrgId:       data.OrgID,
				ClinicianId: data.ClinicianID,
				EntryId:     data.EntryID,
				TesteeId:    data.TesteeID,
				OccurredAt:  timestamppb.New(data.OccurredAt),
			})
		case domainStatistics.EventTypeFootprintTesteeProfileCreated:
			var data domainStatistics.FootprintTesteeProfileCreatedData
			env, err := ParseEventData(payload, &data)
			if err != nil {
				return fmt.Errorf("failed to parse footprint testee profile created event: %w", err)
			}
			return projectBehaviorEvent(ctx, logger, deps, &pb.ProjectBehaviorEventRequest{
				EventId:     env.ID,
				EventType:   eventType,
				OrgId:       data.OrgID,
				ClinicianId: data.ClinicianID,
				EntryId:     data.EntryID,
				TesteeId:    data.TesteeID,
				OccurredAt:  timestamppb.New(data.OccurredAt),
			})
		case domainStatistics.EventTypeFootprintCareRelationshipEstablished:
			var data domainStatistics.FootprintCareRelationshipEstablishedData
			env, err := ParseEventData(payload, &data)
			if err != nil {
				return fmt.Errorf("failed to parse footprint care relationship established event: %w", err)
			}
			return projectBehaviorEvent(ctx, logger, deps, &pb.ProjectBehaviorEventRequest{
				EventId:     env.ID,
				EventType:   eventType,
				OrgId:       data.OrgID,
				ClinicianId: data.ClinicianID,
				EntryId:     data.EntryID,
				TesteeId:    data.TesteeID,
				OccurredAt:  timestamppb.New(data.OccurredAt),
			})
		case domainStatistics.EventTypeFootprintCareRelationshipTransferred:
			var data domainStatistics.FootprintCareRelationshipTransferredData
			env, err := ParseEventData(payload, &data)
			if err != nil {
				return fmt.Errorf("failed to parse footprint care relationship transferred event: %w", err)
			}
			return projectBehaviorEvent(ctx, logger, deps, &pb.ProjectBehaviorEventRequest{
				EventId:           env.ID,
				EventType:         eventType,
				OrgId:             data.OrgID,
				ClinicianId:       data.ToClinicianID,
				SourceClinicianId: data.FromClinicianID,
				TesteeId:          data.TesteeID,
				OccurredAt:        timestamppb.New(data.OccurredAt),
			})
		case domainStatistics.EventTypeFootprintAnswerSheetSubmitted:
			var data domainStatistics.FootprintAnswerSheetSubmittedData
			env, err := ParseEventData(payload, &data)
			if err != nil {
				return fmt.Errorf("failed to parse footprint answersheet submitted event: %w", err)
			}
			return projectBehaviorEvent(ctx, logger, deps, &pb.ProjectBehaviorEventRequest{
				EventId:       env.ID,
				EventType:     eventType,
				OrgId:         data.OrgID,
				TesteeId:      data.TesteeID,
				AnswersheetId: data.AnswerSheetID,
				OccurredAt:    timestamppb.New(data.OccurredAt),
			})
		case domainStatistics.EventTypeFootprintAssessmentCreated:
			var data domainStatistics.FootprintAssessmentCreatedData
			env, err := ParseEventData(payload, &data)
			if err != nil {
				return fmt.Errorf("failed to parse footprint assessment created event: %w", err)
			}
			return projectBehaviorEvent(ctx, logger, deps, &pb.ProjectBehaviorEventRequest{
				EventId:       env.ID,
				EventType:     eventType,
				OrgId:         data.OrgID,
				TesteeId:      data.TesteeID,
				AnswersheetId: data.AnswerSheetID,
				AssessmentId:  data.AssessmentID,
				OccurredAt:    timestamppb.New(data.OccurredAt),
			})
		case domainStatistics.EventTypeFootprintReportGenerated:
			var data domainStatistics.FootprintReportGeneratedData
			env, err := ParseEventData(payload, &data)
			if err != nil {
				return fmt.Errorf("failed to parse footprint report generated event: %w", err)
			}
			return projectBehaviorEvent(ctx, logger, deps, &pb.ProjectBehaviorEventRequest{
				EventId:      env.ID,
				EventType:    eventType,
				OrgId:        data.OrgID,
				TesteeId:     data.TesteeID,
				AssessmentId: data.AssessmentID,
				ReportId:     data.ReportID,
				OccurredAt:   timestamppb.New(data.OccurredAt),
			})
		default:
			return fmt.Errorf("unsupported behavior event type %q", eventType)
		}
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
