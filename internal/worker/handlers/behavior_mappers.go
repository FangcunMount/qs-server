package handlers

import (
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventpayload"
)

type behaviorEventMapper func(payload []byte) (*pb.ProjectBehaviorEventRequest, error)

var behaviorEventMappers = map[string]behaviorEventMapper{
	eventcatalog.FootprintEntryOpened:                 mapFootprintEntryOpened,
	eventcatalog.FootprintIntakeConfirmed:             mapFootprintIntakeConfirmed,
	eventcatalog.FootprintTesteeProfileCreated:        mapFootprintTesteeProfileCreated,
	eventcatalog.FootprintCareRelationshipEstablished: mapFootprintCareRelationshipEstablished,
	eventcatalog.FootprintCareRelationshipTransferred: mapFootprintCareRelationshipTransferred,
	eventcatalog.FootprintAnswerSheetSubmitted:        mapFootprintAnswerSheetSubmitted,
	eventcatalog.FootprintAssessmentCreated:           mapFootprintAssessmentCreated,
	eventcatalog.FootprintReportGenerated:             mapFootprintReportGenerated,
}

func mapFootprintEntryOpened(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
	var data eventpayload.FootprintEntryOpenedData
	env, data, err := parseFootprintPayload(payload, "entry opened", &data)
	if err != nil {
		return nil, err
	}
	req := newBehaviorRequest(env, eventcatalog.FootprintEntryOpened, data.OrgID, data.OccurredAt)
	req.ClinicianId = data.ClinicianID
	req.EntryId = data.EntryID
	return req, nil
}

func mapFootprintIntakeConfirmed(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
	var data eventpayload.FootprintIntakeConfirmedData
	env, data, err := parseFootprintPayload(payload, "intake confirmed", &data)
	if err != nil {
		return nil, err
	}
	req := newBehaviorRequest(env, eventcatalog.FootprintIntakeConfirmed, data.OrgID, data.OccurredAt)
	req.ClinicianId = data.ClinicianID
	req.EntryId = data.EntryID
	req.TesteeId = data.TesteeID
	return req, nil
}

func mapFootprintTesteeProfileCreated(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
	var data eventpayload.FootprintTesteeProfileCreatedData
	env, data, err := parseFootprintPayload(payload, "testee profile created", &data)
	if err != nil {
		return nil, err
	}
	req := newBehaviorRequest(env, eventcatalog.FootprintTesteeProfileCreated, data.OrgID, data.OccurredAt)
	req.ClinicianId = data.ClinicianID
	req.EntryId = data.EntryID
	req.TesteeId = data.TesteeID
	return req, nil
}

func mapFootprintCareRelationshipEstablished(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
	var data eventpayload.FootprintCareRelationshipEstablishedData
	env, data, err := parseFootprintPayload(payload, "care relationship established", &data)
	if err != nil {
		return nil, err
	}
	req := newBehaviorRequest(env, eventcatalog.FootprintCareRelationshipEstablished, data.OrgID, data.OccurredAt)
	req.ClinicianId = data.ClinicianID
	req.EntryId = data.EntryID
	req.TesteeId = data.TesteeID
	return req, nil
}

func mapFootprintCareRelationshipTransferred(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
	var data eventpayload.FootprintCareRelationshipTransferredData
	env, data, err := parseFootprintPayload(payload, "care relationship transferred", &data)
	if err != nil {
		return nil, err
	}
	req := newBehaviorRequest(env, eventcatalog.FootprintCareRelationshipTransferred, data.OrgID, data.OccurredAt)
	req.ClinicianId = data.ToClinicianID
	req.SourceClinicianId = data.FromClinicianID
	req.TesteeId = data.TesteeID
	return req, nil
}

func mapFootprintAnswerSheetSubmitted(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
	var data eventpayload.FootprintAnswerSheetSubmittedData
	env, data, err := parseFootprintPayload(payload, "answersheet submitted", &data)
	if err != nil {
		return nil, err
	}
	req := newBehaviorRequest(env, eventcatalog.FootprintAnswerSheetSubmitted, data.OrgID, data.OccurredAt)
	req.TesteeId = data.TesteeID
	req.AnswersheetId = data.AnswerSheetID
	return req, nil
}

func mapFootprintAssessmentCreated(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
	var data eventpayload.FootprintAssessmentCreatedData
	env, data, err := parseFootprintPayload(payload, "assessment created", &data)
	if err != nil {
		return nil, err
	}
	req := newBehaviorRequest(env, eventcatalog.FootprintAssessmentCreated, data.OrgID, data.OccurredAt)
	req.TesteeId = data.TesteeID
	req.AnswersheetId = data.AnswerSheetID
	req.AssessmentId = data.AssessmentID
	return req, nil
}

func mapFootprintReportGenerated(payload []byte) (*pb.ProjectBehaviorEventRequest, error) {
	var data eventpayload.FootprintReportGeneratedData
	env, data, err := parseFootprintPayload(payload, "report generated", &data)
	if err != nil {
		return nil, err
	}
	req := newBehaviorRequest(env, eventcatalog.FootprintReportGenerated, data.OrgID, data.OccurredAt)
	req.TesteeId = data.TesteeID
	req.AssessmentId = data.AssessmentID
	req.ReportId = data.ReportID
	return req, nil
}
