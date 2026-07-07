package statistics

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventpayload"
	"github.com/FangcunMount/qs-server/internal/pkg/footprintevent"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// BehaviorAggregateType 是re-exported 从 中性 footprint integration。
// event kernel; footprint events 是 共享 contracts, 不 statistics-private。
const BehaviorAggregateType = footprintevent.AggregateType

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

// The constructors below delegate 到 中性 footprintevent kernel so that。
// existing statistics 消费rs keep 稳定 API while schema 是 owned 按。
// 共享 kernel。

func NewFootprintEntryOpenedEvent(orgID int64, clinicianID, entryID uint64, occurredAt time.Time) FootprintEntryOpenedEvent {
	return footprintevent.NewFootprintEntryOpenedEvent(orgID, clinicianID, entryID, occurredAt)
}

func NewFootprintIntakeConfirmedEvent(orgID int64, clinicianID, entryID, testeeID uint64, occurredAt time.Time) FootprintIntakeConfirmedEvent {
	return footprintevent.NewFootprintIntakeConfirmedEvent(orgID, clinicianID, entryID, testeeID, occurredAt)
}

func NewFootprintTesteeProfileCreatedEvent(orgID int64, clinicianID, entryID, testeeID uint64, occurredAt time.Time) FootprintTesteeProfileCreatedEvent {
	return footprintevent.NewFootprintTesteeProfileCreatedEvent(orgID, clinicianID, entryID, testeeID, occurredAt)
}

func NewFootprintCareRelationshipEstablishedEvent(orgID int64, clinicianID, entryID, testeeID uint64, occurredAt time.Time) FootprintCareRelationshipEstablishedEvent {
	return footprintevent.NewFootprintCareRelationshipEstablishedEvent(orgID, clinicianID, entryID, testeeID, occurredAt)
}

func NewFootprintCareRelationshipTransferredEvent(orgID int64, fromClinicianID, toClinicianID, testeeID uint64, occurredAt time.Time) FootprintCareRelationshipTransferredEvent {
	return footprintevent.NewFootprintCareRelationshipTransferredEvent(orgID, fromClinicianID, toClinicianID, testeeID, occurredAt)
}

func NewFootprintAnswerSheetSubmittedEvent(orgID int64, testeeID, answerSheetID uint64, occurredAt time.Time) FootprintAnswerSheetSubmittedEvent {
	return footprintevent.NewFootprintAnswerSheetSubmittedEvent(orgID, testeeID, answerSheetID, occurredAt)
}

func NewFootprintAssessmentCreatedEvent(orgID int64, testeeID, answerSheetID, assessmentID uint64, occurredAt time.Time) FootprintAssessmentCreatedEvent {
	return footprintevent.NewFootprintAssessmentCreatedEvent(orgID, testeeID, answerSheetID, assessmentID, occurredAt)
}

func NewFootprintReportGeneratedEvent(orgID int64, testeeID, assessmentID, reportID uint64, occurredAt time.Time) FootprintReportGeneratedEvent {
	return footprintevent.NewFootprintReportGeneratedEvent(orgID, testeeID, assessmentID, reportID, occurredAt)
}
