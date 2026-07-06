package reporting

import (
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
)

// ErrWriterNotConfigured reports a missing interpretation writer dependency.
func ErrWriterNotConfigured() error {
	return evalerrors.ModuleNotConfigured("interpretation writer is not configured")
}

// NewInterpretationWriter creates a writer for the interpretation phase after scoring.
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
