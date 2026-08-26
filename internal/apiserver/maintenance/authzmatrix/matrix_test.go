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
			"101": {RoleAdmin}, "102": {RoleEvaluator}, "103": {RolePlanManager}, "104": {RoleStaff},
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
	if !evidence.Passed || evidence.PolicyVersion != 42 || len(evidence.Cases) != 12 || len(evidence.Subjects) != 4 {
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
}

func TestRunnerFailsClosedOnMatrixMismatch(t *testing.T) {
	t.Parallel()

	checker := matrixChecker{override: map[string]appauthz.ObjectDecision{
		"user:102/plan": {Allowed: true, MatchedRole: RoleEvaluator, PolicyVersion: 42},
	}}
	runner := NewRunner(staticSubjects(testSubjects()), staticSnapshots{
		"101": {RoleAdmin}, "102": {RoleEvaluator}, "103": {RolePlanManager}, "104": {RoleStaff},
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
		"101": {RoleAdmin}, "102": {RoleEvaluator, RoleAdmin}, "103": {RolePlanManager}, "104": {RoleStaff},
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
		{Kind: "evaluator", ExpectedRole: RoleEvaluator, UserID: "102"},
		{Kind: "plan_manager", ExpectedRole: RolePlanManager, UserID: "103"},
		{Kind: "other", ExpectedRole: RoleStaff, UserID: "104"},
	}
}

type staticSubjects []Subject

func (s staticSubjects) Load(context.Context) ([]Subject, error) {
	return append([]Subject(nil), s...), nil
}

type staticSnapshots map[string][]string

func (s staticSnapshots) Load(_ context.Context, domain, userID string) (*appauthz.Snapshot, error) {
	roles := append([]string(nil), s[userID]...)
	return &appauthz.Snapshot{DirectRoles: roles, EffectiveRoles: append([]string(nil), roles...), AuthzVersion: 42, AuthorizationDomain: domain}, nil
}

type matrixChecker struct {
	override map[string]appauthz.ObjectDecision
}

func (c matrixChecker) CheckObject(_ context.Context, request appauthz.ObjectCheckRequest) (appauthz.ObjectDecision, error) {
	origin := "missing"
	if attribute, ok := request.Attributes[appauthz.ObjectOriginTypeAttribute]; ok && attribute.String != nil {
		origin = *attribute.String
	}
	if decision, ok := c.override[request.Subject+"/"+origin]; ok {
		return decision, nil
	}
	role := map[string]string{"user:101": RoleAdmin, "user:102": RoleEvaluator, "user:103": RolePlanManager, "user:104": RoleStaff}[request.Subject]
	decision := appauthz.ObjectDecision{PolicyVersion: 42}
	switch {
	case role == RoleAdmin:
		decision.Allowed = true
		decision.MatchedRole = RoleAdmin
		decision.MatchedGrantID = "grant-admin"
	case role == RoleEvaluator && origin == "adhoc":
		decision.Allowed = true
		decision.MatchedRole = RoleEvaluator
		decision.MatchedGrantID = "grant-evaluator"
	case role == RolePlanManager && origin == "plan":
		decision.Allowed = true
		decision.MatchedRole = RolePlanManager
		decision.MatchedGrantID = "grant-plan-manager"
	case origin == "missing" && (role == RoleEvaluator || role == RolePlanManager):
		decision.DenyCode = "attribute_missing"
		decision.MissingAttributeKeys = []string{appauthz.ObjectOriginTypeAttribute}
	default:
		decision.DenyCode = "policy_not_matched"
	}
	return decision, nil
}
