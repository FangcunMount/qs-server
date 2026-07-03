package statistics

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventpayload"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const BehaviorAggregateType = "BehaviorFootprint"

const (
	EventTypeFootprintEntryOpened                 = eventcatalog.FootprintEntryOpened
	EventTypeFootprintIntakeConfirmed             = eventcatalog.FootprintIntakeConfirmed
	EventTypeFootprintTesteeProfileCreated        = eventcatalog.FootprintTesteeProfileCreated
	EventTypeFootprintCareRelationshipEstablished = eventcatalog.FootprintCareRelationshipEstablished
	EventTypeFootprintCareRelationshipTransferred = eventcatalog.FootprintCareRelationshipTransferred
	EventTypeFootprintAnswerSheetSubmitted        = eventcatalog.FootprintAnswerSheetSubmitted
	EventTypeFootprintAssessmentCreated           = eventcatalog.FootprintAssessmentCreated
	EventTypeFootprintReportGenerated             = eventcatalog.FootprintReportGenerated
)

type FootprintEntryOpenedData = eventpayload.FootprintEntryOpenedData
type FootprintIntakeConfirmedData = eventpayload.FootprintIntakeConfirmedData
type FootprintTesteeProfileCreatedData = eventpayload.FootprintTesteeProfileCreatedData
type FootprintCareRelationshipEstablishedData = eventpayload.FootprintCareRelationshipEstablishedData
type FootprintCareRelationshipTransferredData = eventpayload.FootprintCareRelationshipTransferredData
type FootprintAnswerSheetSubmittedData = eventpayload.FootprintAnswerSheetSubmittedData
type FootprintAssessmentCreatedData = eventpayload.FootprintAssessmentCreatedData
type FootprintReportGeneratedData = eventpayload.FootprintReportGeneratedData

type FootprintEntryOpenedEvent = event.Event[FootprintEntryOpenedData]
type FootprintIntakeConfirmedEvent = event.Event[FootprintIntakeConfirmedData]
type FootprintTesteeProfileCreatedEvent = event.Event[FootprintTesteeProfileCreatedData]
type FootprintCareRelationshipEstablishedEvent = event.Event[FootprintCareRelationshipEstablishedData]
type FootprintCareRelationshipTransferredEvent = event.Event[FootprintCareRelationshipTransferredData]
type FootprintAnswerSheetSubmittedEvent = event.Event[FootprintAnswerSheetSubmittedData]
type FootprintAssessmentCreatedEvent = event.Event[FootprintAssessmentCreatedData]
type FootprintReportGeneratedEvent = event.Event[FootprintReportGeneratedData]

func NewFootprintEntryOpenedEvent(orgID int64, clinicianID, entryID uint64, occurredAt time.Time) FootprintEntryOpenedEvent {
	return event.New(EventTypeFootprintEntryOpened, BehaviorAggregateType, strconv.FormatUint(entryID, 10), FootprintEntryOpenedData{
		OrgID:       orgID,
		ClinicianID: clinicianID,
		EntryID:     entryID,
		OccurredAt:  occurredAt,
	})
}

func NewFootprintIntakeConfirmedEvent(orgID int64, clinicianID, entryID, testeeID uint64, occurredAt time.Time) FootprintIntakeConfirmedEvent {
	return event.New(EventTypeFootprintIntakeConfirmed, BehaviorAggregateType, strconv.FormatUint(testeeID, 10), FootprintIntakeConfirmedData{
		OrgID:       orgID,
		ClinicianID: clinicianID,
		EntryID:     entryID,
		TesteeID:    testeeID,
		OccurredAt:  occurredAt,
	})
}

func NewFootprintTesteeProfileCreatedEvent(orgID int64, clinicianID, entryID, testeeID uint64, occurredAt time.Time) FootprintTesteeProfileCreatedEvent {
	return event.New(EventTypeFootprintTesteeProfileCreated, BehaviorAggregateType, strconv.FormatUint(testeeID, 10), FootprintTesteeProfileCreatedData{
		OrgID:       orgID,
		ClinicianID: clinicianID,
		EntryID:     entryID,
		TesteeID:    testeeID,
		OccurredAt:  occurredAt,
	})
}

func NewFootprintCareRelationshipEstablishedEvent(orgID int64, clinicianID, entryID, testeeID uint64, occurredAt time.Time) FootprintCareRelationshipEstablishedEvent {
	return event.New(EventTypeFootprintCareRelationshipEstablished, BehaviorAggregateType, strconv.FormatUint(testeeID, 10), FootprintCareRelationshipEstablishedData{
		OrgID:       orgID,
		ClinicianID: clinicianID,
		EntryID:     entryID,
		TesteeID:    testeeID,
		OccurredAt:  occurredAt,
	})
}

func NewFootprintCareRelationshipTransferredEvent(orgID int64, fromClinicianID, toClinicianID, testeeID uint64, occurredAt time.Time) FootprintCareRelationshipTransferredEvent {
	return event.New(EventTypeFootprintCareRelationshipTransferred, BehaviorAggregateType, strconv.FormatUint(testeeID, 10), FootprintCareRelationshipTransferredData{
		OrgID:           orgID,
		FromClinicianID: fromClinicianID,
		ToClinicianID:   toClinicianID,
		TesteeID:        testeeID,
		OccurredAt:      occurredAt,
	})
}

func NewFootprintAnswerSheetSubmittedEvent(orgID int64, testeeID, answerSheetID uint64, occurredAt time.Time) FootprintAnswerSheetSubmittedEvent {
	return event.New(EventTypeFootprintAnswerSheetSubmitted, BehaviorAggregateType, strconv.FormatUint(answerSheetID, 10), FootprintAnswerSheetSubmittedData{
		OrgID:         orgID,
		TesteeID:      testeeID,
		AnswerSheetID: answerSheetID,
		OccurredAt:    occurredAt,
	})
}

func NewFootprintAssessmentCreatedEvent(orgID int64, testeeID, answerSheetID, assessmentID uint64, occurredAt time.Time) FootprintAssessmentCreatedEvent {
	return event.New(EventTypeFootprintAssessmentCreated, BehaviorAggregateType, strconv.FormatUint(assessmentID, 10), FootprintAssessmentCreatedData{
		OrgID:         orgID,
		TesteeID:      testeeID,
		AnswerSheetID: answerSheetID,
		AssessmentID:  assessmentID,
		OccurredAt:    occurredAt,
	})
}

func NewFootprintReportGeneratedEvent(orgID int64, testeeID, assessmentID, reportID uint64, occurredAt time.Time) FootprintReportGeneratedEvent {
	return event.New(EventTypeFootprintReportGenerated, BehaviorAggregateType, strconv.FormatUint(reportID, 10), FootprintReportGeneratedData{
		OrgID:        orgID,
		TesteeID:     testeeID,
		AssessmentID: assessmentID,
		ReportID:     reportID,
		OccurredAt:   occurredAt,
	})
}
