package historicalseed

import (
	"fmt"
	"time"

	commonpb "github.com/FangcunMount/qs-server/api/grpc/gen/common"
)

// ToProto converts the verified in-process context into the explicit internal
// gRPC carrier. Nil means the ordinary, system-time execution path.
func ToProto(ctx Context) *commonpb.HistoricalExecutionContext {
	return &commonpb.HistoricalExecutionContext{
		BatchId: ctx.BatchID, ScenarioId: ctx.ScenarioID, OrgId: ctx.OrgID, Version: uint32(ctx.Version),
		Timeline: &commonpb.BusinessTimeline{
			TesteeCreatedAt: formatTime(ctx.Timeline.TesteeCreatedAt),
			EntryResolvedAt: formatTime(ctx.Timeline.EntryResolvedAt), EntryIntakeAt: formatTime(ctx.Timeline.EntryIntakeAt),
			EnrollmentJoinedAt: formatTime(ctx.Timeline.EnrollmentJoinedAt), TaskOpenedAt: formatTime(ctx.Timeline.TaskOpenedAt),
			TaskCompletedAt: formatTime(ctx.Timeline.TaskCompletedAt), AnswersheetFilledAt: formatTime(ctx.Timeline.AnswerSheetFilledAt),
			AssessmentCreatedAt: formatTime(ctx.Timeline.AssessmentCreatedAt), AssessmentSubmittedAt: formatTime(ctx.Timeline.AssessmentSubmittedAt),
			EvaluatedAt: formatTime(ctx.Timeline.EvaluatedAt), ReportGeneratedAt: formatTime(ctx.Timeline.ReportGeneratedAt),
		},
	}
}

// FromProto parses an internal gRPC carrier. Date-range authorization happens
// at the signed REST boundary; this conversion still enforces version,
// identity and ordering so malformed internal calls fail closed.
func FromProto(in *commonpb.HistoricalExecutionContext) (Context, error) {
	if in == nil {
		return Context{}, nil
	}
	timeline := in.GetTimeline()
	if timeline == nil {
		return Context{}, fmt.Errorf("historical execution timeline is required")
	}
	parse := func(name, raw string) (*time.Time, error) {
		if raw == "" {
			return nil, nil
		}
		value, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return nil, fmt.Errorf("parse historical %s: %w", name, err)
		}
		return &value, nil
	}
	var out Context
	out.BatchID, out.ScenarioID, out.OrgID, out.Version = in.GetBatchId(), in.GetScenarioId(), in.GetOrgId(), int(in.GetVersion())
	fields := []struct {
		name string
		raw  string
		set  func(*time.Time)
	}{
		{"testee_created_at", timeline.GetTesteeCreatedAt(), func(v *time.Time) { out.Timeline.TesteeCreatedAt = v }},
		{"entry_resolved_at", timeline.GetEntryResolvedAt(), func(v *time.Time) { out.Timeline.EntryResolvedAt = v }},
		{"entry_intake_at", timeline.GetEntryIntakeAt(), func(v *time.Time) { out.Timeline.EntryIntakeAt = v }},
		{"enrollment_joined_at", timeline.GetEnrollmentJoinedAt(), func(v *time.Time) { out.Timeline.EnrollmentJoinedAt = v }},
		{"task_opened_at", timeline.GetTaskOpenedAt(), func(v *time.Time) { out.Timeline.TaskOpenedAt = v }},
		{"task_completed_at", timeline.GetTaskCompletedAt(), func(v *time.Time) { out.Timeline.TaskCompletedAt = v }},
		{"answersheet_filled_at", timeline.GetAnswersheetFilledAt(), func(v *time.Time) { out.Timeline.AnswerSheetFilledAt = v }},
		{"assessment_created_at", timeline.GetAssessmentCreatedAt(), func(v *time.Time) { out.Timeline.AssessmentCreatedAt = v }},
		{"assessment_submitted_at", timeline.GetAssessmentSubmittedAt(), func(v *time.Time) { out.Timeline.AssessmentSubmittedAt = v }},
		{"evaluated_at", timeline.GetEvaluatedAt(), func(v *time.Time) { out.Timeline.EvaluatedAt = v }},
		{"report_generated_at", timeline.GetReportGeneratedAt(), func(v *time.Time) { out.Timeline.ReportGeneratedAt = v }},
	}
	for _, field := range fields {
		value, err := parse(field.name, field.raw)
		if err != nil {
			return Context{}, err
		}
		field.set(value)
	}
	if err := out.Validate(time.Time{}, time.Time{}, time.UTC); err != nil {
		return Context{}, err
	}
	return out, nil
}

func formatTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339Nano)
}
