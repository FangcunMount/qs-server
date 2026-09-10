package authz

import (
	"context"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Role isolation matrix for the independent-v1 cutover. Assertions are action
// facts only; role labels are fixtures that match the IAM production matrix.
func TestIndependentRolePermissionMatrix(t *testing.T) {

	operator := &Snapshot{
		DirectRoles: []string{"qs:assessment_operator"},
		Permissions: []Permission{
			{Resource: AssessmentResource, Action: "list_progress", Mode: AuthorizationModeUnconditional},
			{Resource: AssessmentResource, Action: "read_progress", Mode: AuthorizationModeUnconditional},
			{Resource: AssessmentResource, Action: "batch_evaluate", Mode: AuthorizationModeUnconditional},
		},
	}
	reviewer := &Snapshot{
		DirectRoles: []string{"qs:result_reviewer"},
		Permissions: []Permission{
			{Resource: AssessmentResource, Action: "read", Mode: AuthorizationModeUnconditional},
			{Resource: AssessmentResource, Action: "list", Mode: AuthorizationModeUnconditional},
			{Resource: AssessmentResource, Action: "statistics", Mode: AuthorizationModeUnconditional},
			{Resource: AnswerSheetResource, Action: "read", Mode: AuthorizationModeUnconditional},
			{Resource: AnswerSheetResource, Action: "list", Mode: AuthorizationModeUnconditional},
			{Resource: AnswerSheetResource, Action: "statistics", Mode: AuthorizationModeUnconditional},
			{Resource: "qs:evaluation:collection:reports", Action: "read", Mode: AuthorizationModeUnconditional},
			{Resource: "qs:evaluation:collection:reports", Action: "list", Mode: AuthorizationModeUnconditional},
		},
	}
	planManager := &Snapshot{
		DirectRoles: []string{"qs:evaluation_plan_manager"},
		Permissions: []Permission{
			{Resource: EvaluationPlanResource, Action: "create", Mode: AuthorizationModeUnconditional},
			{Resource: EvaluationPlanTaskResource, Action: "schedule", Mode: AuthorizationModeUnconditional},
			{Resource: AssessmentResource, Action: "read_progress", Mode: AuthorizationModeUnconditional},
			{Resource: AssessmentResource, Action: "list_progress", Mode: AuthorizationModeUnconditional},
		},
	}
	contentManager := &Snapshot{
		DirectRoles: []string{"qs:content_manager"},
		Permissions: []Permission{
			{Resource: QuestionnaireResource, Action: "create", Mode: AuthorizationModeUnconditional},
			{Resource: AssessmentModelResource, Action: "read", Mode: AuthorizationModeUnconditional},
		},
	}
	bothJobs := &Snapshot{
		DirectRoles: []string{"qs:assessment_operator", "qs:result_reviewer"},
		Permissions: append(append([]Permission{}, operator.Permissions...), reviewer.Permissions...),
	}

	type check struct {
		resource string
		action   string
		via      string // "result" uses RequirePermission; "perm" uses RequirePermission
		allow    bool
	}
	cases := []struct {
		name     string
		snapshot *Snapshot
		checks   []check
	}{
		{
			name:     "operator_progress_and_batch_without_results",
			snapshot: operator,
			checks: []check{
				{AssessmentResource, "list_progress", "perm", true},
				{AssessmentResource, "read_progress", "perm", true},
				{AssessmentResource, "batch_evaluate", "result", true},
				{AssessmentResource, "read", "result", false},
				{AssessmentResource, "list", "result", false},
				{AnswerSheetResource, "read", "result", false},
				{"qs:evaluation:collection:reports", "read", "result", false},
			},
		},
		{
			name:     "result_reviewer_results_without_execution",
			snapshot: reviewer,
			checks: []check{
				{AssessmentResource, "read", "result", true},
				{AssessmentResource, "list", "result", true},
				{AssessmentResource, "statistics", "result", true},
				{AnswerSheetResource, "read", "result", true},
				{"qs:evaluation:collection:reports", "list", "result", true},
				{AssessmentResource, "batch_evaluate", "result", false},
				{AssessmentResource, "list_progress", "perm", false},
				{EvaluationPlanResource, "create", "perm", false},
			},
		},
		{
			name:     "plan_manager_plans_without_reports",
			snapshot: planManager,
			checks: []check{
				{AssessmentResource, "list_progress", "perm", true},
				{EvaluationPlanResource, "create", "perm", true},
				{"qs:evaluation:collection:reports", "read", "result", false},
				{AssessmentResource, "batch_evaluate", "result", false},
			},
		},
		{
			name:     "content_manager_without_people_or_results",
			snapshot: contentManager,
			checks: []check{
				{QuestionnaireResource, "create", "perm", true},
				{AssessmentResource, "list_progress", "perm", false},
				{AssessmentResource, "read", "result", false},
				{AnswerSheetResource, "list", "result", false},
			},
		},
		{
			name:     "dual_role_union",
			snapshot: bothJobs,
			checks: []check{
				{AssessmentResource, "batch_evaluate", "result", true},
				{AssessmentResource, "read", "result", true},
				{"qs:evaluation:collection:reports", "read", "result", true},
				{AssessmentResource, "list_progress", "perm", true},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := WithSnapshot(context.Background(), tc.snapshot)
			for _, item := range tc.checks {
				var err error
				switch item.via {
				case "result":
					err = RequirePermission(ctx, item.resource, item.action)
				default:
					err = RequirePermission(ctx, item.resource, item.action)
				}
				allowed := err == nil
				if allowed != item.allow {
					t.Fatalf("%s %s/%s allowed=%v want=%v err=%v", item.via, item.resource, item.action, allowed, item.allow, err)
				}
				if !item.allow && !cberrors.IsCode(err, code.ErrPermissionDenied) {
					t.Fatalf("%s %s/%s error = %v, want permission denied", item.via, item.resource, item.action, err)
				}
			}
		})
	}
}

func TestLegacyRoleModelCannotBypassResultGate(t *testing.T) {
	t.Setenv("QS_AUTHZ_ROLE_MODEL", "legacy")
	ctx := WithSnapshot(context.Background(), &Snapshot{DirectRoles: []string{"qs:assessment_operator"}})
	if err := RequirePermission(ctx, AssessmentResource, "read"); !cberrors.IsCode(err, code.ErrPermissionDenied) {
		t.Fatalf("legacy environment must not bypass permission: %v", err)
	}
}

func TestObsoleteRoleModelDoesNotChangePermission(t *testing.T) {
	t.Setenv("QS_AUTHZ_ROLE_MODEL", "not-a-model")
	ctx := WithSnapshot(context.Background(), &Snapshot{
		Permissions: []Permission{{Resource: AssessmentResource, Action: "read", Mode: AuthorizationModeUnconditional}},
	})
	err := RequirePermission(ctx, AssessmentResource, "read")
	if err != nil {
		t.Fatalf("obsolete configuration must not change valid permissions: %v", err)
	}
}
