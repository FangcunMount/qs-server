package statistics

import (
	"fmt"
	"time"
)

const (
	ScanSourceEntryResolve = "entry_resolve_log"
	ScanSourceEntryIntake  = "entry_intake_log"
	ScanSourceAnswerSheet  = "answersheet"
	ScanSourceAssessment   = "assessment"
	ScanSourceReport       = "report"
)

const (
	ScanWatermarkStatusIdle    = "idle"
	ScanWatermarkStatusRunning = "running"
	ScanWatermarkStatusFailed  = "failed"
)

// ScanWatermark tracks incremental scan progress for a source/org pair.
type ScanWatermark struct {
	ID              uint64
	SourceName      string
	OrgID           int64
	LastSeenID      uint64
	LastSeenTime    *time.Time
	ScanWindowStart *time.Time
	ScanWindowEnd   *time.Time
	Status          string
	LastError       string
}

// EntryResolveFact is a scan source row from assessment_entry_resolve_log.
type EntryResolveFact struct {
	OrgID       int64
	ClinicianID uint64
	EntryID     uint64
	LogID       uint64
	OccurredAt  time.Time
}

// EntryIntakeFact is a scan source row from assessment_entry_intake_log.
type EntryIntakeFact struct {
	OrgID             int64
	ClinicianID       uint64
	EntryID           uint64
	TesteeID          uint64
	LogID             uint64
	TesteeCreated     bool
	AssignmentCreated bool
	OccurredAt        time.Time
}

// AnswerSheetSubmittedFact is a scan source row for submitted answer sheets.
type AnswerSheetSubmittedFact struct {
	OrgID         int64
	TesteeID      uint64
	AnswerSheetID uint64
	OccurredAt    time.Time
}

// AssessmentCreatedFact is a scan source row for created assessments.
type AssessmentCreatedFact struct {
	OrgID         int64
	TesteeID      uint64
	AnswerSheetID uint64
	AssessmentID  uint64
	OccurredAt    time.Time
}

// ReportGeneratedFact is a scan source row for generated reports.
type ReportGeneratedFact struct {
	OrgID        int64
	TesteeID     uint64
	AssessmentID uint64
	ReportID     uint64
	OccurredAt   time.Time
}

// ScanBehaviorFootprintID returns a stable footprint ID for scan projections.
func ScanBehaviorFootprintID(eventName BehaviorEventName, sourceID uint64) string {
	return fmt.Sprintf("scan:%s:%d", eventName, sourceID)
}
