package handlers

import (
	"context"
	"fmt"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func handleBehaviorProjector(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		if deps.InternalClient == nil {
			return fmt.Errorf("internal client is not available")
		}

		switch eventType {
		case domainStatistics.EventTypeFootprintEntryOpened:
			var data domainStatistics.FootprintEntryOpenedData
			env, err := ParseEventData(payload, &data)
			if err != nil {
				return fmt.Errorf("failed to parse footprint entry opened event: %w", err)
			}
			return projectBehaviorEvent(ctx, deps, &pb.ProjectBehaviorEventRequest{
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
			return projectBehaviorEvent(ctx, deps, &pb.ProjectBehaviorEventRequest{
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
			return projectBehaviorEvent(ctx, deps, &pb.ProjectBehaviorEventRequest{
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
			return projectBehaviorEvent(ctx, deps, &pb.ProjectBehaviorEventRequest{
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
			return projectBehaviorEvent(ctx, deps, &pb.ProjectBehaviorEventRequest{
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
			return projectBehaviorEvent(ctx, deps, &pb.ProjectBehaviorEventRequest{
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
			return projectBehaviorEvent(ctx, deps, &pb.ProjectBehaviorEventRequest{
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
			return projectBehaviorEvent(ctx, deps, &pb.ProjectBehaviorEventRequest{
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

func projectBehaviorEvent(ctx context.Context, deps *Dependencies, req *pb.ProjectBehaviorEventRequest) error {
	resp, err := deps.InternalClient.ProjectBehaviorEvent(ctx, req)
	if err != nil {
		return err
	}
	if resp == nil {
		return fmt.Errorf("behavior projector returned empty response")
	}
	return nil
}
