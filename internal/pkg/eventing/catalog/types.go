package eventcatalog

// Event type constants are the code-level mirror of configs/events.yaml.
const (
	QuestionnaireChanged = "questionnaire.changed"

	AnswerSheetSubmitted = "answersheet.submitted"

	EvaluationRequested        = "evaluation.requested"
	EvaluationRetryRequested   = "evaluation.retry.requested"
	EvaluationOutcomeCommitted = "evaluation.outcome.committed"
	EvaluationFailed           = "evaluation.failed"

	InterpretationReportGenerated = "interpretation.report.generated"
	InterpretationReportFailed    = "interpretation.report.failed"
	InterpretationRetryRequested  = "interpretation.retry.requested"

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
		EvaluationRetryRequested,
		EvaluationOutcomeCommitted,
		EvaluationFailed,
		InterpretationReportGenerated,
		InterpretationReportFailed,
		InterpretationRetryRequested,
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
