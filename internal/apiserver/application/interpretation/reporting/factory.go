package reporting

import (
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
)

// ErrWriterNotConfigured 报告缺失 解释写入器 dependency。
func ErrWriterNotConfigured() error {
	return evalerrors.ModuleNotConfigured("interpretation writer is not configured")
}

// NewInterpretationWriter 创建writer 用于 interpretation phase 在之后 计分。
func NewInterpretationWriter(
	assessmentRepo assessment.Repository,
	scoreProjectors ScoreProjectorRegistry,
	reportBuilders ReportBuilderRegistry,
	reportSaver ReportDurableSaver,
	notifier CompletionNotifier,
	reportStatus *reportstatus.Reporter,
) (Writer, error) {
	return NewWriter(assessmentRepo, scoreProjectors, reportBuilders, reportSaver, notifier, reportStatus)
}
