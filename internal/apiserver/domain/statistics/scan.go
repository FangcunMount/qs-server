package statistics

import (
	"fmt"
	"time"
)

const (
	ScanSourceAnswerSheet = "answersheet"
	ScanSourceReport      = "report"
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

// AnswerSheetSubmittedFact is a scan source row for submitted answer sheets.
type AnswerSheetSubmittedFact struct {
	OrgID         int64
	TesteeID      uint64
	AnswerSheetID uint64
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
