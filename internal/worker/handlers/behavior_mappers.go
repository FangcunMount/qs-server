package handlers

import (
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
)

type behaviorEventMapper func(payload []byte) (*pb.ProjectBehaviorEventRequest, error)

var behaviorEventMappers = map[string]behaviorEventMapper{
	domainStatistics.EventTypeFootprintEntryOpened:                 mapFootprintEntryOpened,
	domainStatistics.EventTypeFootprintIntakeConfirmed:             mapFootprintIntakeConfirmed,
	domainStatistics.EventTypeFootprintTesteeProfileCreated:        mapFootprintTesteeProfileCreated,
	domainStatistics.EventTypeFootprintCareRelationshipEstablished: mapFootprintCareRelationshipEstablished,
	domainStatistics.EventTypeFootprintCareRelationshipTransferred: mapFootprintCareRelationshipTransferred,
	domainStatistics.EventTypeFootprintAnswerSheetSubmitted:        mapFootprintAnswerSheetSubmitted,
	domainStatistics.EventTypeFootprintAssessmentCreated:           mapFootprintAssessmentCreated,
	domainStatistics.EventTypeFootprintReportGenerated:             mapFootprintReportGenerated,
}

func mapFootprintEntryOpened(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
	var data domainStatistics.FootprintEntryOpenedData
	env, data, err := parseFootprintPayload(payload, "entry opened", &data)
	if err != nil {
		return nil, err
	}
	req := newBehaviorRequest(env, domainStatistics.EventTypeFootprintEntryOpened, data.OrgID, data.OccurredAt)
	req.ClinicianId = data.ClinicianID
	req.EntryId = data.EntryID
	return req, nil
}

func mapFootprintIntakeConfirmed(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
	var data domainStatistics.FootprintIntakeConfirmedData
	env, data, err := parseFootprintPayload(payload, "intake confirmed", &data)
	if err != nil {
		return nil, err
	}
	req := newBehaviorRequest(env, domainStatistics.EventTypeFootprintIntakeConfirmed, data.OrgID, data.OccurredAt)
	req.ClinicianId = data.ClinicianID
	req.EntryId = data.EntryID
	req.TesteeId = data.TesteeID
	return req, nil
}

func mapFootprintTesteeProfileCreated(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
	var data domainStatistics.FootprintTesteeProfileCreatedData
	env, data, err := parseFootprintPayload(payload, "testee profile created", &data)
	if err != nil {
		return nil, err
	}
	req := newBehaviorRequest(env, domainStatistics.EventTypeFootprintTesteeProfileCreated, data.OrgID, data.OccurredAt)
	req.ClinicianId = data.ClinicianID
	req.EntryId = data.EntryID
	req.TesteeId = data.TesteeID
	return req, nil
}

func mapFootprintCareRelationshipEstablished(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
	var data domainStatistics.FootprintCareRelationshipEstablishedData
	env, data, err := parseFootprintPayload(payload, "care relationship established", &data)
	if err != nil {
		return nil, err
	}
	req := newBehaviorRequest(env, domainStatistics.EventTypeFootprintCareRelationshipEstablished, data.OrgID, data.OccurredAt)
	req.ClinicianId = data.ClinicianID
	req.EntryId = data.EntryID
	req.TesteeId = data.TesteeID
	return req, nil
}

func mapFootprintCareRelationshipTransferred(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
	var data domainStatistics.FootprintCareRelationshipTransferredData
	env, data, err := parseFootprintPayload(payload, "care relationship transferred", &data)
	if err != nil {
		return nil, err
	}
	req := newBehaviorRequest(env, domainStatistics.EventTypeFootprintCareRelationshipTransferred, data.OrgID, data.OccurredAt)
	req.ClinicianId = data.ToClinicianID
	req.SourceClinicianId = data.FromClinicianID
	req.TesteeId = data.TesteeID
	return req, nil
}

func mapFootprintAnswerSheetSubmitted(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
	var data domainStatistics.FootprintAnswerSheetSubmittedData
	env, data, err := parseFootprintPayload(payload, "answersheet submitted", &data)
	if err != nil {
		return nil, err
	}
	req := newBehaviorRequest(env, domainStatistics.EventTypeFootprintAnswerSheetSubmitted, data.OrgID, data.OccurredAt)
	req.TesteeId = data.TesteeID
	req.AnswersheetId = data.AnswerSheetID
	return req, nil
}

func mapFootprintAssessmentCreated(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
	var data domainStatistics.FootprintAssessmentCreatedData
	env, data, err := parseFootprintPayload(payload, "assessment created", &data)
	if err != nil {
		return nil, err
	}
	req := newBehaviorRequest(env, domainStatistics.EventTypeFootprintAssessmentCreated, data.OrgID, data.OccurredAt)
	req.TesteeId = data.TesteeID
	req.AnswersheetId = data.AnswerSheetID
	req.AssessmentId = data.AssessmentID
	return req, nil
}

func mapFootprintReportGenerated(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
	var data domainStatistics.FootprintReportGeneratedData
	env, data, err := parseFootprintPayload(payload, "report generated", &data)
	if err != nil {
		return nil, err
	}
	req := newBehaviorRequest(env, domainStatistics.EventTypeFootprintReportGenerated, data.OrgID, data.OccurredAt)
	req.TesteeId = data.TesteeID
	req.AssessmentId = data.AssessmentID
	req.ReportId = data.ReportID
	return req, nil
}
