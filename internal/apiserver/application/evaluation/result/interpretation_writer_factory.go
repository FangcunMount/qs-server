package result

import (
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
)

// NewInterpretationWriter creates a writer for the interpretation phase after scoring.
func NewInterpretationWriter(
	assessmentRepo assessment.Repository,
	scoreProjectors ScoreProjectorRegistry,
	reportBuilders ReportBuilderRegistry,
	reportSaver ReportDurableSaver,
	notifier CompletionNotifier,
	reportStatus *reportstatus.Reporter,
) (InterpretationWriter, error) {
	return interpretationreporting.NewInterpretationWriter(
		assessmentRepo,
		scoreProjectors,
		reportBuilders,
		reportSaver,
		notifier,
		reportStatus,
	)
}
