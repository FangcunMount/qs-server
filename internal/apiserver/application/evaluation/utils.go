package evaluation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
)

// ReportStatusIDs 获取测评报告状态的 assessmentID 和 answerSheetID
func ReportStatusIDs(a *assessment.Assessment) (assessmentID, answerSheetID string) {
	if a == nil {
		return "", ""
	}
	assessmentID = reportstatus.AssessmentKey(a.ID().Uint64())
	if ref := a.AnswerSheetRef(); !ref.IsEmpty() {
		answerSheetID = reportstatus.AssessmentKey(ref.ID().Uint64())
	}
	return assessmentID, answerSheetID
}
