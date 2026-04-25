package eventcatalog

// Event type constants are the code-level mirror of configs/events.yaml.
const (
	QuestionnaireChanged = "questionnaire.changed"

	AnswerSheetSubmitted = "answersheet.submitted"

	AssessmentSubmitted   = "assessment.submitted"
	AssessmentInterpreted = "assessment.interpreted"
	AssessmentFailed      = "assessment.failed"

	ReportGenerated = "report.generated"

	FootprintEntryOpened                 = "footprint.entry_opened"
	FootprintIntakeConfirmed             = "footprint.intake_confirmed"
	FootprintTesteeProfileCreated        = "footprint.testee_profile_created"
	FootprintCareRelationshipEstablished = "footprint.care_relationship_established"
	FootprintCareRelationshipTransferred = "footprint.care_relationship_transferred"
	FootprintAnswerSheetSubmitted        = "footprint.answersheet_submitted"
	FootprintAssessmentCreated           = "footprint.assessment_created"
	FootprintReportGenerated             = "footprint.report_generated"

	ScaleChanged = "scale.changed"

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
		AssessmentSubmitted,
		AssessmentInterpreted,
		AssessmentFailed,
		ReportGenerated,
		FootprintEntryOpened,
		FootprintIntakeConfirmed,
		FootprintTesteeProfileCreated,
		FootprintCareRelationshipEstablished,
		FootprintCareRelationshipTransferred,
		FootprintAnswerSheetSubmitted,
		FootprintAssessmentCreated,
		FootprintReportGenerated,
		ScaleChanged,
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
