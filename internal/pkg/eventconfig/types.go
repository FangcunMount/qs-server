package eventconfig

import "github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"

const (
	QuestionnaireChanged = eventcatalog.QuestionnaireChanged

	AnswerSheetSubmitted = eventcatalog.AnswerSheetSubmitted

	AssessmentSubmitted   = eventcatalog.AssessmentSubmitted
	AssessmentInterpreted = eventcatalog.AssessmentInterpreted
	AssessmentFailed      = eventcatalog.AssessmentFailed

	ReportGenerated = eventcatalog.ReportGenerated

	FootprintEntryOpened                 = eventcatalog.FootprintEntryOpened
	FootprintIntakeConfirmed             = eventcatalog.FootprintIntakeConfirmed
	FootprintTesteeProfileCreated        = eventcatalog.FootprintTesteeProfileCreated
	FootprintCareRelationshipEstablished = eventcatalog.FootprintCareRelationshipEstablished
	FootprintCareRelationshipTransferred = eventcatalog.FootprintCareRelationshipTransferred
	FootprintAnswerSheetSubmitted        = eventcatalog.FootprintAnswerSheetSubmitted
	FootprintAssessmentCreated           = eventcatalog.FootprintAssessmentCreated
	FootprintReportGenerated             = eventcatalog.FootprintReportGenerated

	ScaleChanged = eventcatalog.ScaleChanged

	TaskOpened    = eventcatalog.TaskOpened
	TaskCompleted = eventcatalog.TaskCompleted
	TaskExpired   = eventcatalog.TaskExpired
	TaskCanceled  = eventcatalog.TaskCanceled
)

// EventTypes returns all event types known by code.
func EventTypes() []string {
	return eventcatalog.EventTypes()
}

// ValidateEventTypes returns code-level event types missing from the catalog.
func ValidateEventTypes(cfg *Config) []string {
	return eventcatalog.ValidateEventTypes(cfg)
}
