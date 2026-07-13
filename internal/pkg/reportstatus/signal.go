package reportstatus

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/signalcatalog"
)

const SignalNameReportStatusChanged = signalcatalog.ReportStatusChanged

// ChangedSignal 报告状态变更唤醒信号（best-effort，非业务事实）。
type ChangedSignal struct {
	AssessmentID  string    `json:"assessment_id"`
	AnswerSheetID string    `json:"answer_sheet_id,omitempty"`
	ReportID      string    `json:"report_id,omitempty"`
	Status        string    `json:"status"`
	Stage         string    `json:"stage,omitempty"`
	Reason        string    `json:"reason,omitempty"`
	Message       string    `json:"message,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
}

func (s ChangedSignal) SignalName() string {
	return SignalNameReportStatusChanged
}

func (s ChangedSignal) SignalKey() string {
	return s.AssessmentID
}
