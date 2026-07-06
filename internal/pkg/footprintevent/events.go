// Package footprintevent centralizes the behavior-footprint integration events.
//
// Footprint events are integration/contract events with multiple producers
// (survey, evaluation, ...) consumed by the statistics read side. Their schema
// (event types in eventcatalog, payloads in eventpayload) already lives in the
// neutral shared kernel, so their constructors belong here too — not inside any
// single bounded context. This lets producers emit footprints without importing
// the statistics module.
package footprintevent

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventpayload"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// AggregateType is the aggregate type shared by all footprint events.
const AggregateType = "BehaviorFootprint"

func NewFootprintEntryOpenedEvent(orgID int64, clinicianID, entryID uint64, occurredAt time.Time) event.Event[eventpayload.FootprintEntryOpenedData] {
	return event.New(eventcatalog.FootprintEntryOpened, AggregateType, strconv.FormatUint(entryID, 10), eventpayload.FootprintEntryOpenedData{
		OrgID:       orgID,
		ClinicianID: clinicianID,
		EntryID:     entryID,
		OccurredAt:  occurredAt,
	})
}

func NewFootprintIntakeConfirmedEvent(orgID int64, clinicianID, entryID, testeeID uint64, occurredAt time.Time) event.Event[eventpayload.FootprintIntakeConfirmedData] {
	return event.New(eventcatalog.FootprintIntakeConfirmed, AggregateType, strconv.FormatUint(testeeID, 10), eventpayload.FootprintIntakeConfirmedData{
		OrgID:       orgID,
		ClinicianID: clinicianID,
		EntryID:     entryID,
		TesteeID:    testeeID,
		OccurredAt:  occurredAt,
	})
}

func NewFootprintTesteeProfileCreatedEvent(orgID int64, clinicianID, entryID, testeeID uint64, occurredAt time.Time) event.Event[eventpayload.FootprintTesteeProfileCreatedData] {
	return event.New(eventcatalog.FootprintTesteeProfileCreated, AggregateType, strconv.FormatUint(testeeID, 10), eventpayload.FootprintTesteeProfileCreatedData{
		OrgID:       orgID,
		ClinicianID: clinicianID,
		EntryID:     entryID,
		TesteeID:    testeeID,
		OccurredAt:  occurredAt,
	})
}

func NewFootprintCareRelationshipEstablishedEvent(orgID int64, clinicianID, entryID, testeeID uint64, occurredAt time.Time) event.Event[eventpayload.FootprintCareRelationshipEstablishedData] {
	return event.New(eventcatalog.FootprintCareRelationshipEstablished, AggregateType, strconv.FormatUint(testeeID, 10), eventpayload.FootprintCareRelationshipEstablishedData{
		OrgID:       orgID,
		ClinicianID: clinicianID,
		EntryID:     entryID,
		TesteeID:    testeeID,
		OccurredAt:  occurredAt,
	})
}

func NewFootprintCareRelationshipTransferredEvent(orgID int64, fromClinicianID, toClinicianID, testeeID uint64, occurredAt time.Time) event.Event[eventpayload.FootprintCareRelationshipTransferredData] {
	return event.New(eventcatalog.FootprintCareRelationshipTransferred, AggregateType, strconv.FormatUint(testeeID, 10), eventpayload.FootprintCareRelationshipTransferredData{
		OrgID:           orgID,
		FromClinicianID: fromClinicianID,
		ToClinicianID:   toClinicianID,
		TesteeID:        testeeID,
		OccurredAt:      occurredAt,
	})
}

func NewFootprintAnswerSheetSubmittedEvent(orgID int64, testeeID, answerSheetID uint64, occurredAt time.Time) event.Event[eventpayload.FootprintAnswerSheetSubmittedData] {
	return event.New(eventcatalog.FootprintAnswerSheetSubmitted, AggregateType, strconv.FormatUint(answerSheetID, 10), eventpayload.FootprintAnswerSheetSubmittedData{
		OrgID:         orgID,
		TesteeID:      testeeID,
		AnswerSheetID: answerSheetID,
		OccurredAt:    occurredAt,
	})
}

func NewFootprintAssessmentCreatedEvent(orgID int64, testeeID, answerSheetID, assessmentID uint64, occurredAt time.Time) event.Event[eventpayload.FootprintAssessmentCreatedData] {
	return event.New(eventcatalog.FootprintAssessmentCreated, AggregateType, strconv.FormatUint(assessmentID, 10), eventpayload.FootprintAssessmentCreatedData{
		OrgID:         orgID,
		TesteeID:      testeeID,
		AnswerSheetID: answerSheetID,
		AssessmentID:  assessmentID,
		OccurredAt:    occurredAt,
	})
}

func NewFootprintReportGeneratedEvent(orgID int64, testeeID, assessmentID, reportID uint64, occurredAt time.Time) event.Event[eventpayload.FootprintReportGeneratedData] {
	return event.New(eventcatalog.FootprintReportGenerated, AggregateType, strconv.FormatUint(reportID, 10), eventpayload.FootprintReportGeneratedData{
		OrgID:        orgID,
		TesteeID:     testeeID,
		AssessmentID: assessmentID,
		ReportID:     reportID,
		OccurredAt:   occurredAt,
	})
}
