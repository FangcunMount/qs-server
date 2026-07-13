package handlers

// NewRegistry returns the worker's explicit handler factory catalog.
func NewRegistry() *Registry {
	return newRegistryFromFactories(map[string]HandlerFactory{
		"answersheet_submitted_handler": func(deps *Dependencies) HandlerFunc {
			return handleAnswerSheetSubmitted(deps)
		},
		"evaluation_requested_handler": func(deps *Dependencies) HandlerFunc {
			return handleEvaluationRequested(deps)
		},
		"evaluation_outcome_committed_handler": func(deps *Dependencies) HandlerFunc {
			return handleEvaluationOutcomeCommitted(deps)
		},
		"evaluation_failed_handler": func(deps *Dependencies) HandlerFunc {
			return handleEvaluationFailed(deps)
		},
		"questionnaire_changed_handler": func(deps *Dependencies) HandlerFunc {
			return handleQuestionnaireChanged(deps)
		},
		"interpretation_report_generated_handler": func(deps *Dependencies) HandlerFunc {
			return handleInterpretationReportGenerated(deps)
		},
		"interpretation_report_failed_handler": func(deps *Dependencies) HandlerFunc {
			return handleInterpretationReportFailed(deps)
		},
		"assessment_model_changed_handler": func(deps *Dependencies) HandlerFunc {
			return handleAssessmentModelChanged(deps)
		},
		"task_opened_handler": func(deps *Dependencies) HandlerFunc {
			return handleTaskOpened(deps)
		},
		"task_completed_handler": func(deps *Dependencies) HandlerFunc {
			return handleTaskCompleted(deps)
		},
		"task_expired_handler": func(deps *Dependencies) HandlerFunc {
			return handleTaskExpired(deps)
		},
		"task_canceled_handler": func(deps *Dependencies) HandlerFunc {
			return handleTaskCanceled(deps)
		},
	})
}
