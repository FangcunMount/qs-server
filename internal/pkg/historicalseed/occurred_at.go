package historicalseed

import (
	"context"
	"fmt"
	"time"
)

type Stage string

const (
	StageTesteeCreated       Stage = "testee_created"
	StageEntryResolved       Stage = "entry_resolved"
	StageEntryIntake         Stage = "entry_intake"
	StageEnrollmentJoined    Stage = "enrollment_joined"
	StageTaskOpened          Stage = "task_opened"
	StageTaskCompleted       Stage = "task_completed"
	StageAnswerSheetFilled   Stage = "answersheet_filled"
	StageAssessmentCreated   Stage = "assessment_created"
	StageAssessmentSubmitted Stage = "assessment_submitted"
	StageEvaluated           Stage = "evaluated"
	StageReportGenerated     Stage = "report_generated"
)

// OccurredAt returns the stage's business occurrence time only for a verified
// context. Ordinary calls keep their supplied system time. Missing historical
// stage time is an error, never an implicit fallback.
func OccurredAt(ctx context.Context, orgID uint64, stage Stage, systemNow time.Time) (time.Time, error) {
	historical, ok := FromContext(ctx)
	if !ok {
		return systemNow, nil
	}
	if err := historical.ValidateOrg(orgID); err != nil {
		return time.Time{}, err
	}
	var occurredAt *time.Time
	switch stage {
	case StageTesteeCreated:
		occurredAt = historical.Timeline.TesteeCreatedAt
	case StageEntryResolved:
		occurredAt = historical.Timeline.EntryResolvedAt
	case StageEntryIntake:
		occurredAt = historical.Timeline.EntryIntakeAt
	case StageEnrollmentJoined:
		occurredAt = historical.Timeline.EnrollmentJoinedAt
	case StageTaskOpened:
		occurredAt = historical.Timeline.TaskOpenedAt
	case StageTaskCompleted:
		occurredAt = historical.Timeline.TaskCompletedAt
	case StageAnswerSheetFilled:
		occurredAt = historical.Timeline.AnswerSheetFilledAt
	case StageAssessmentCreated:
		occurredAt = historical.Timeline.AssessmentCreatedAt
	case StageAssessmentSubmitted:
		occurredAt = historical.Timeline.AssessmentSubmittedAt
	case StageEvaluated:
		occurredAt = historical.Timeline.EvaluatedAt
	case StageReportGenerated:
		occurredAt = historical.Timeline.ReportGeneratedAt
	default:
		return time.Time{}, fmt.Errorf("unsupported historical seed stage %q", stage)
	}
	if occurredAt == nil || occurredAt.IsZero() {
		return time.Time{}, fmt.Errorf("historical seed stage %s has no business time", stage)
	}
	return *occurredAt, nil
}
