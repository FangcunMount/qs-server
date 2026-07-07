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

// ScanWatermark 跟踪incremental scan progress 用于 来源/org pair。
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

// EntryResolveFact 是scan 来源 行 从 assessment_entry_resolve_log。
type EntryResolveFact struct {
	OrgID       int64
	ClinicianID uint64
	EntryID     uint64
	LogID       uint64
	OccurredAt  time.Time
}

// EntryIntakeFact 是scan 来源 行 从 assessment_entry_intake_log。
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

// AnswerSheetSubmittedFact 是scan 来源 行 用于 submitted 答卷。
type AnswerSheetSubmittedFact struct {
	OrgID         int64
	TesteeID      uint64
	AnswerSheetID uint64
	OccurredAt    time.Time
}

// AssessmentCreatedFact 是scan 来源 行 用于 created assessments。
type AssessmentCreatedFact struct {
	OrgID         int64
	TesteeID      uint64
	AnswerSheetID uint64
	AssessmentID  uint64
	OccurredAt    time.Time
}

// ReportGeneratedFact 是scan 来源 行 用于 generated reports。
type ReportGeneratedFact struct {
	OrgID        int64
	TesteeID     uint64
	AssessmentID uint64
	ReportID     uint64
	OccurredAt   time.Time
}

// ScanBehaviorFootprintID 返回稳定 footprint ID 用于 scan 投影。
func ScanBehaviorFootprintID(eventName BehaviorEventName, sourceID uint64) string {
	return fmt.Sprintf("scan:%s:%d", eventName, sourceID)
}
