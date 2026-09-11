package authzmatrix

import (
	"context"
	"strings"
	"testing"
	"time"

	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

func TestRunnerExecutesProductionRoleOriginMatrix(t *testing.T) {
	t.Parallel()

	subjects := testSubjects()
	runner := NewRunner(
		staticSubjects(subjects),
		staticSnapshots{
			"101": {RoleAdmin}, "102": {RoleAssessmentOperator}, "103": {RolePlanManager}, "104": {RoleResultReviewer},
		},
		matrixChecker{},
		"0123456789abcdef",
		"qs-apiserver.svc",
	)
	runner.now = func() time.Time { return time.Date(2026, 8, 26, 1, 2, 3, 0, time.UTC) }

	evidence, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !evidence.Passed || evidence.PolicyVersion != 42 || len(evidence.Cases) != 6 || len(evidence.Subjects) != 4 {
		t.Fatalf("Run() evidence = %+v", evidence)
	}
	for _, subject := range evidence.Subjects {
		if len(subject.SubjectFingerprint) != 16 || subject.SubjectFingerprint == "101" || subject.SubjectFingerprint == "102" || subject.SubjectFingerprint == "103" || subject.SubjectFingerprint == "104" {
			t.Fatalf("unsafe subject evidence = %+v", subject)
		}
	}
	for _, testCase := range evidence.Cases {
		if !testCase.Passed || testCase.PolicyVersion != 42 {
			t.Fatalf("failed case = %+v", testCase)
		}
	}
	wantScenarios := map[string]int{"retry": 4, "force_retry": 2}
	for _, testCase := range evidence.Cases {
		wantScenarios[testCase.Scenario]--
	}
	for scenario, remaining := range wantScenarios {
		if remaining != 0 {
			t.Fatalf("scenario %s remaining count = %d", scenario, remaining)
		}
	}
}

func TestRunnerFailsClosedOnMatrixMismatch(t *testing.T) {
	t.Parallel()

	checker := matrixChecker{override: map[string]appauthz.ActionDecision{
		"user:102/retry": {Allowed: false, MatchedRole: RoleAssessmentOperator, PolicyVersion: 42},
	}}
	runner := NewRunner(staticSubjects(testSubjects()), staticSnapshots{
		"101": {RoleAdmin}, "102": {RoleAssessmentOperator}, "103": {RolePlanManager}, "104": {RoleResultReviewer},
	}, checker, "commit", "qs-apiserver.svc")

	evidence, err := runner.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "matrix mismatch") {
		t.Fatalf("Run() error = %v", err)
	}
	if evidence.Passed {
		t.Fatalf("Run() unexpectedly passed: %+v", evidence)
	}
}

func TestRunnerRejectsConflictingResolvedRole(t *testing.T) {
	t.Parallel()

	runner := NewRunner(staticSubjects(testSubjects()), staticSnapshots{
		"101": {RoleAdmin}, "102": {RoleAssessmentOperator, RoleAdmin}, "103": {RolePlanManager}, "104": {RoleResultReviewer},
	}, matrixChecker{}, "commit", "qs-apiserver.svc")

	_, err := runner.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "conflicting role") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRunnerRequiresTrustedServiceIdentity(t *testing.T) {
	t.Parallel()
	runner := NewRunner(staticSubjects(testSubjects()), staticSnapshots{}, matrixChecker{}, "commit", "admin")
	if _, err := runner.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "unexpected service identity") {
		t.Fatalf("Run() error = %v", err)
	}
}

func testSubjects() []Subject {
	return []Subject{
		{Kind: "admin", ExpectedRole: RoleAdmin, UserID: "101"},
		{Kind: "operator", ExpectedRole: RoleAssessmentOperator, UserID: "102"},
		{Kind: "plan_manager", ExpectedRole: RolePlanManager, UserID: "103"},
		{Kind: "other", ExpectedRole: RoleResultReviewer, UserID: "104"},
	}
}

type staticSubjects []Subject

func (s staticSubjects) Load(context.Context) ([]Subject, error) {
	return append([]Subject(nil), s...), nil
}

type staticSnapshots map[string][]string

func (s staticSnapshots) Load(_ context.Context, userID string) (*appauthz.Snapshot, error) {
	roles := append([]string(nil), s[userID]...)
	return &appauthz.Snapshot{DirectRoles: roles, EffectiveRoles: append([]string(nil), roles...), AuthzVersion: 42}, nil
}

type matrixChecker struct {
	override map[string]appauthz.ActionDecision
}

func (c matrixChecker) CheckAction(_ context.Context, r appauthz.ActionCheckRequest) (appauthz.ActionDecision, error) {
	if override, ok := c.override[r.Subject+"/"+r.Action]; ok {
		return override, nil
	}
	role := map[string]string{"user:101": RoleAdmin, "user:102": RoleAssessmentOperator, "user:103": RolePlanManager}[r.Subject]
	allowed := role != "" && (r.Action == "retry" || role == RoleAdmin)
	d := appauthz.ActionDecision{Allowed: allowed, PolicyVersion: 42}
	if allowed {
		d.MatchedRole = role
		d.MatchedGrantID = "grant"
	} else {
		d.DenyCode = "policy_not_matched"
	}
	return d, nil
}

func TestRunnerChecksAdministratorGlobalMatchEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, matchedRole, grantID string
		wantPass                   bool
	}{
		{"another assigned role", "platform_admin", "grant-platform", true},
		{"missing role evidence", "", "grant-unknown", false},
		{"missing grant evidence", "platform_admin", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checker := matrixChecker{override: map[string]appauthz.ActionDecision{
				"user:101/retry": {Allowed: true, MatchedRole: tc.matchedRole, MatchedGrantID: tc.grantID, PolicyVersion: 42},
			}}
			runner := NewRunner(staticSubjects(testSubjects()), staticSnapshots{
				"101": {RoleAdmin}, "102": {RoleAssessmentOperator}, "103": {RolePlanManager}, "104": {RoleResultReviewer},
			}, checker, "commit", "qs-apiserver.svc")
			evidence, err := runner.Run(context.Background())
			if (err == nil && evidence.Passed) != tc.wantPass {
				t.Fatalf("passed=%v err=%v", evidence.Passed, err)
			}
		})
	}
}
