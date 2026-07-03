package handlers

import (
	"fmt"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type behaviorEventMapper func(payload []byte) (*pb.ProjectBehaviorEventRequest, error)

var behaviorEventMappers = map[string]behaviorEventMapper{
	domainStatistics.EventTypeFootprintEntryOpened: func(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
		var data domainStatistics.FootprintEntryOpenedData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse footprint entry opened event: %w", err)
		}
		return &pb.ProjectBehaviorEventRequest{
			EventId:     env.ID,
			EventType:   domainStatistics.EventTypeFootprintEntryOpened,
			OrgId:       data.OrgID,
			ClinicianId: data.ClinicianID,
			EntryId:     data.EntryID,
			OccurredAt:  timestamppb.New(data.OccurredAt),
		}, nil
	},
	domainStatistics.EventTypeFootprintIntakeConfirmed: func(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
		var data domainStatistics.FootprintIntakeConfirmedData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse footprint intake confirmed event: %w", err)
		}
		return &pb.ProjectBehaviorEventRequest{
			EventId:     env.ID,
			EventType:   domainStatistics.EventTypeFootprintIntakeConfirmed,
			OrgId:       data.OrgID,
			ClinicianId: data.ClinicianID,
			EntryId:     data.EntryID,
			TesteeId:    data.TesteeID,
			OccurredAt:  timestamppb.New(data.OccurredAt),
		}, nil
	},
	domainStatistics.EventTypeFootprintTesteeProfileCreated: func(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
		var data domainStatistics.FootprintTesteeProfileCreatedData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse footprint testee profile created event: %w", err)
		}
		return &pb.ProjectBehaviorEventRequest{
			EventId:     env.ID,
			EventType:   domainStatistics.EventTypeFootprintTesteeProfileCreated,
			OrgId:       data.OrgID,
			ClinicianId: data.ClinicianID,
			EntryId:     data.EntryID,
			TesteeId:    data.TesteeID,
			OccurredAt:  timestamppb.New(data.OccurredAt),
		}, nil
	},
	domainStatistics.EventTypeFootprintCareRelationshipEstablished: func(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
		var data domainStatistics.FootprintCareRelationshipEstablishedData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse footprint care relationship established event: %w", err)
		}
		return &pb.ProjectBehaviorEventRequest{
			EventId:     env.ID,
			EventType:   domainStatistics.EventTypeFootprintCareRelationshipEstablished,
			OrgId:       data.OrgID,
			ClinicianId: data.ClinicianID,
			EntryId:     data.EntryID,
			TesteeId:    data.TesteeID,
			OccurredAt:  timestamppb.New(data.OccurredAt),
		}, nil
	},
	domainStatistics.EventTypeFootprintCareRelationshipTransferred: func(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
		var data domainStatistics.FootprintCareRelationshipTransferredData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse footprint care relationship transferred event: %w", err)
		}
		return &pb.ProjectBehaviorEventRequest{
			EventId:           env.ID,
			EventType:         domainStatistics.EventTypeFootprintCareRelationshipTransferred,
			OrgId:             data.OrgID,
			ClinicianId:       data.ToClinicianID,
			SourceClinicianId: data.FromClinicianID,
			TesteeId:          data.TesteeID,
			OccurredAt:        timestamppb.New(data.OccurredAt),
		}, nil
	},
	domainStatistics.EventTypeFootprintAnswerSheetSubmitted: func(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
		var data domainStatistics.FootprintAnswerSheetSubmittedData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse footprint answersheet submitted event: %w", err)
		}
		return &pb.ProjectBehaviorEventRequest{
			EventId:       env.ID,
			EventType:     domainStatistics.EventTypeFootprintAnswerSheetSubmitted,
			OrgId:         data.OrgID,
			TesteeId:      data.TesteeID,
			AnswersheetId: data.AnswerSheetID,
			OccurredAt:    timestamppb.New(data.OccurredAt),
		}, nil
	},
	domainStatistics.EventTypeFootprintAssessmentCreated: func(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
		var data domainStatistics.FootprintAssessmentCreatedData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse footprint assessment created event: %w", err)
		}
		return &pb.ProjectBehaviorEventRequest{
			EventId:       env.ID,
			EventType:     domainStatistics.EventTypeFootprintAssessmentCreated,
			OrgId:         data.OrgID,
			TesteeId:      data.TesteeID,
			AnswersheetId: data.AnswerSheetID,
			AssessmentId:  data.AssessmentID,
			OccurredAt:    timestamppb.New(data.OccurredAt),
		}, nil
	},
	domainStatistics.EventTypeFootprintReportGenerated: func(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
		var data domainStatistics.FootprintReportGeneratedData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse footprint report generated event: %w", err)
		}
		return &pb.ProjectBehaviorEventRequest{
			EventId:      env.ID,
			EventType:    domainStatistics.EventTypeFootprintReportGenerated,
			OrgId:        data.OrgID,
			TesteeId:     data.TesteeID,
			AssessmentId: data.AssessmentID,
			ReportId:     data.ReportID,
			OccurredAt:   timestamppb.New(data.OccurredAt),
		}, nil
	},
}
