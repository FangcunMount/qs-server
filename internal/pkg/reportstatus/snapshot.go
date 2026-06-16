package reportstatus

import "time"

const DefaultTTL = 48 * time.Hour

// Snapshot Redis 中的报告等待状态缓存。
type Snapshot struct {
	AssessmentID  string    `json:"assessment_id"`
	AnswerSheetID string    `json:"answer_sheet_id,omitempty"`
	ReportID      string    `json:"report_id,omitempty"`
	Status        string    `json:"status"`
	Stage         string    `json:"stage,omitempty"`
	Message       string    `json:"message,omitempty"`
	Reason        string    `json:"reason,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
	Version       int64     `json:"version,omitempty"`
}
