package statistics

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
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

type FootprintEntryOpenedData struct {
	OrgID       int64     `json:"org_id"`
	ClinicianID uint64    `json:"clinician_id"`
	EntryID     uint64    `json:"entry_id"`
	OccurredAt  time.Time `json:"occurred_at"`
}

type FootprintIntakeConfirmedData struct {
	OrgID       int64     `json:"org_id"`
	ClinicianID uint64    `json:"clinician_id"`
	EntryID     uint64    `json:"entry_id"`
	TesteeID    uint64    `json:"testee_id"`
	OccurredAt  time.Time `json:"occurred_at"`
}

type FootprintTesteeProfileCreatedData struct {
	OrgID       int64     `json:"org_id"`
	ClinicianID uint64    `json:"clinician_id"`
	EntryID     uint64    `json:"entry_id"`
	TesteeID    uint64    `json:"testee_id"`
	OccurredAt  time.Time `json:"occurred_at"`
}

type FootprintCareRelationshipEstablishedData struct {
	OrgID       int64     `json:"org_id"`
	ClinicianID uint64    `json:"clinician_id"`
	EntryID     uint64    `json:"entry_id"`
	TesteeID    uint64    `json:"testee_id"`
	OccurredAt  time.Time `json:"occurred_at"`
}

type FootprintCareRelationshipTransferredData struct {
	OrgID           int64     `json:"org_id"`
	FromClinicianID uint64    `json:"from_clinician_id"`
	ToClinicianID   uint64    `json:"to_clinician_id"`
	TesteeID        uint64    `json:"testee_id"`
	OccurredAt      time.Time `json:"occurred_at"`
}

type FootprintAnswerSheetSubmittedData struct {
	OrgID         int64     `json:"org_id"`
	TesteeID      uint64    `json:"testee_id"`
	AnswerSheetID uint64    `json:"answersheet_id"`
	OccurredAt    time.Time `json:"occurred_at"`
}

type FootprintAssessmentCreatedData struct {
	OrgID         int64     `json:"org_id"`
	TesteeID      uint64    `json:"testee_id"`
	AnswerSheetID uint64    `json:"answersheet_id"`
	AssessmentID  uint64    `json:"assessment_id"`
	OccurredAt    time.Time `json:"occurred_at"`
}

type FootprintReportGeneratedData struct {
	OrgID        int64     `json:"org_id"`
	TesteeID     uint64    `json:"testee_id"`
	AssessmentID uint64    `json:"assessment_id"`
	ReportID     uint64    `json:"report_id"`
	OccurredAt   time.Time `json:"occurred_at"`
}

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
