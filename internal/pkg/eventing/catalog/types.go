package eventcatalog

// Event type constants are the code-level mirror of configs/events.yaml.
const (
	QuestionnaireChanged = "questionnaire.changed"

	AnswerSheetSubmitted = "answersheet.submitted"

	EvaluationRequested        = "evaluation.requested"
	EvaluationOutcomeCommitted = "evaluation.outcome.committed"
	EvaluationFailed           = "evaluation.failed"

	InterpretationReportGenerated = "interpretation.report.generated"
	InterpretationReportFailed    = "interpretation.report.failed"

	// Deprecated identifiers retained only while repository-wide tests and
	// operational fixtures migrate. They resolve to the new event contract and
	// do not preserve any old wire event names.
	AssessmentSubmitted          = EvaluationRequested
	AssessmentEvaluated          = EvaluationOutcomeCommitted
	AssessmentInterpreted        = InterpretationReportGenerated
	AssessmentInterpretedOutcome = InterpretationReportGenerated
	AssessmentFailed             = EvaluationFailed
	ReportGenerated              = InterpretationReportGenerated
	ReportGeneratedOutcome       = InterpretationReportGenerated

	AssessmentModelChanged = "assessment_model.changed"

	TaskOpened    = "task.opened"
	TaskCompleted = "task.completed"
	TaskExpired   = "task.expired"
	TaskCanceled  = "task.canceled"
)

// EventTypes returns all event types known by code.
func EventTypes() []string {
	return []string{
		QuestionnaireChanged,
		AnswerSheetSubmitted,
		EvaluationRequested,
		EvaluationOutcomeCommitted,
		EvaluationFailed,
		InterpretationReportGenerated,
		InterpretationReportFailed,
		AssessmentModelChanged,
		TaskOpened,
		TaskCompleted,
		TaskExpired,
		TaskCanceled,
	}
}

// ValidateEventTypes returns code-level event types missing from the catalog.
func ValidateEventTypes(cfg *Config) []string {
	var missing []string
	for _, et := range EventTypes() {
		if _, ok := cfg.Events[et]; !ok {
			missing = append(missing, et)
		}
	}
	return missing
}
