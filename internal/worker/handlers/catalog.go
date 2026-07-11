package handlers

// NewRegistry returns the worker's explicit handler factory catalog.
func NewRegistry() *Registry {
	return newRegistryFromFactories(map[string]HandlerFactory{
		"answersheet_submitted_handler": func(deps *Dependencies) HandlerFunc {
			return handleAnswerSheetSubmitted(deps)
		},
		"assessment_submitted_handler": func(deps *Dependencies) HandlerFunc {
			return handleAssessmentSubmitted(deps)
		},
		"assessment_evaluated_handler": func(deps *Dependencies) HandlerFunc {
			return handleAssessmentEvaluated(deps)
		},
		"assessment_interpreted_handler": func(deps *Dependencies) HandlerFunc {
			return handleAssessmentInterpreted(deps)
		},
		"assessment_failed_handler": func(deps *Dependencies) HandlerFunc {
			return handleAssessmentFailed(deps)
		},
		"behavior_projector_handler": func(deps *Dependencies) HandlerFunc {
			return handleBehaviorProjector(deps)
		},
		"questionnaire_changed_handler": func(deps *Dependencies) HandlerFunc {
			return handleQuestionnaireChanged(deps)
		},
		"report_generated_handler": func(deps *Dependencies) HandlerFunc {
			return handleReportGenerated(deps)
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
