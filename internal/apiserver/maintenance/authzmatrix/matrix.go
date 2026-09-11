package authzmatrix

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

const (
	AssessmentResource = "qs:evaluation:collection:assessments"
	RetryAction        = "retry"
	ForceRetryAction   = "force_retry"

	RoleAdmin              = "qs:admin"
	RoleAssessmentOperator = "qs:assessment_operator"
	RolePlanManager        = "qs:evaluation_plan_manager"
	RoleResultReviewer     = "qs:result_reviewer"
)

type Subject struct {
	Kind         string
	ExpectedRole string
	UserID       string
	Source       string
}

type SubjectSource interface {
	Load(context.Context) ([]Subject, error)
}

type SnapshotReader interface {
	Load(context.Context, string) (*appauthz.Snapshot, error)
}

type ActionChecker interface {
	CheckAction(context.Context, appauthz.ActionCheckRequest) (appauthz.ActionDecision, error)
}

type Evidence struct {
	SchemaVersion   string            `json:"schema_version"`
	CheckedAt       time.Time         `json:"checked_at"`
	GitCommit       string            `json:"git_commit"`
	ServiceIdentity string            `json:"service_identity"`
	Resource        string            `json:"resource"`
	Action          string            `json:"action"`
	PolicyVersion   int64             `json:"policy_version"`
	Subjects        []SubjectEvidence `json:"subjects"`
	Cases           []CaseEvidence    `json:"cases"`
	Passed          bool              `json:"passed"`
}

type SubjectEvidence struct {
	Kind               string   `json:"kind"`
	ExpectedRole       string   `json:"expected_role"`
	Source             string   `json:"source"`
	SubjectFingerprint string   `json:"subject_fingerprint"`
	ResolvedRoles      []string `json:"resolved_roles"`
}

type CaseEvidence struct {
	Kind              string `json:"kind"`
	Scenario          string `json:"scenario"`
	Action            string `json:"action"`
	ExpectedAllowed   bool   `json:"expected_allowed"`
	ExpectedErrorCode string `json:"expected_error_code,omitempty"`
	Allowed           bool   `json:"allowed"`
	ErrorCode         string `json:"error_code,omitempty"`
	DenyCode          string `json:"deny_code,omitempty"`
	MatchedRole       string `json:"matched_role,omitempty"`
	MatchedGrantID    string `json:"matched_grant_id,omitempty"`
	PolicyVersion     int64  `json:"policy_version"`
	Passed            bool   `json:"passed"`
}

type Runner struct {
	subjects        SubjectSource
	snapshots       SnapshotReader
	checker         ActionChecker
	now             func() time.Time
	gitCommit       string
	serviceIdentity string
}

func NewRunner(subjects SubjectSource, snapshots SnapshotReader, checker ActionChecker, gitCommit, serviceIdentity string) *Runner {
	return &Runner{
		subjects: subjects, snapshots: snapshots, checker: checker,
		now: time.Now, gitCommit: strings.TrimSpace(gitCommit), serviceIdentity: strings.TrimSpace(serviceIdentity),
	}
}

func (r *Runner) Run(ctx context.Context) (Evidence, error) {
	if r == nil {
		return Evidence{}, errors.New("authz matrix runner is required")
	}
	evidence := Evidence{
		SchemaVersion: "iam-authz-production-matrix/v2",
		CheckedAt:     r.now().UTC(), GitCommit: r.gitCommit, ServiceIdentity: r.serviceIdentity,
		Resource: AssessmentResource, Action: RetryAction,
	}
	if r.subjects == nil || r.snapshots == nil || r.checker == nil {
		return evidence, errors.New("authz matrix dependencies are incomplete")
	}
	if r.serviceIdentity != "qs-apiserver.svc" {
		return evidence, fmt.Errorf("unexpected service identity %q", r.serviceIdentity)
	}

	subjects, err := r.subjects.Load(ctx)
	if err != nil {
		return evidence, fmt.Errorf("load production subjects: %w", err)
	}
	if err := validateSubjects(subjects); err != nil {
		return evidence, err
	}

	versions := make(map[int64]struct{})
	for _, subject := range subjects {
		snapshot, err := r.snapshots.Load(ctx, subject.UserID)
		if err != nil {
			return evidence, fmt.Errorf("load IAM snapshot for %s subject: %w", subject.Kind, err)
		}
		if err := validateResolvedRoles(subject, snapshot.DirectRoleNames()); err != nil {
			return evidence, err
		}
		versions[snapshot.AuthzVersion] = struct{}{}
		roles := snapshot.EffectiveRoleNames()
		sort.Strings(roles)
		evidence.Subjects = append(evidence.Subjects, SubjectEvidence{
			Kind: subject.Kind, ExpectedRole: subject.ExpectedRole,
			Source:             subject.Source,
			SubjectFingerprint: fingerprint(subject.UserID), ResolvedRoles: roles,
		})
	}
	if len(versions) != 1 {
		return evidence, fmt.Errorf("policy version changed while loading matrix subjects: %v", sortedVersions(versions))
	}
	for version := range versions {
		if version <= 0 {
			return evidence, fmt.Errorf("invalid loaded policy version %d", version)
		}
		evidence.PolicyVersion = version
	}

	for _, testCase := range matrixCases(subjects) {
		request := appauthz.ActionCheckRequest{Subject: appauthz.SubjectKey(testCase.subject.UserID), Resource: AssessmentResource, Action: testCase.action}
		decision, err := r.checker.CheckAction(ctx, request)
		errorCode := authorizationErrorCode(err)
		if err != nil && testCase.expectedErrorCode == "" {
			return evidence, fmt.Errorf("check %s/%s: %w", testCase.subject.Kind, testCase.caseName, err)
		}
		if err == nil && testCase.expectedErrorCode != "" {
			return evidence, fmt.Errorf("authorization matrix mismatch for %s/%s: expected error %s", testCase.subject.Kind, testCase.caseName, testCase.expectedErrorCode)
		}
		policyVersion := evidence.PolicyVersion
		if err == nil {
			versions[decision.PolicyVersion] = struct{}{}
			policyVersion = decision.PolicyVersion
		}
		passed := decision.Allowed == testCase.expectedAllowed && errorCode == testCase.expectedErrorCode
		if testCase.expectedDenyCode != "" {
			passed = passed && decision.DenyCode == testCase.expectedDenyCode
		}
		if testCase.expectedMatchedRole != "" {
			matched := decision.MatchedRole == testCase.expectedMatchedRole
			if testCase.subject.Kind == "admin" {
				// 快照仅列出 QS 角色；在线决定可以命中该主体的全局角色。
				// 此处核对允许结果和授权证据，角色归属由 IAM 在线检查保证。
				matched = decision.MatchedRole != ""
			}
			passed = passed && matched && decision.MatchedGrantID != ""
		}
		evidence.Cases = append(evidence.Cases, CaseEvidence{
			Kind: testCase.subject.Kind, Scenario: testCase.scenario, Action: testCase.action,
			ExpectedAllowed: testCase.expectedAllowed, ExpectedErrorCode: testCase.expectedErrorCode,
			Allowed: decision.Allowed, ErrorCode: errorCode,
			DenyCode: decision.DenyCode, MatchedRole: decision.MatchedRole,
			MatchedGrantID: decision.MatchedGrantID,
			PolicyVersion:  policyVersion, Passed: passed,
		})
		if !passed {
			return evidence, fmt.Errorf("authorization matrix mismatch for %s/%s", testCase.subject.Kind, testCase.caseName)
		}
	}

	if len(versions) != 1 {
		return evidence, fmt.Errorf("policy version changed during matrix run: %v", sortedVersions(versions))
	}
	for version := range versions {
		if version <= 0 {
			return evidence, fmt.Errorf("invalid loaded policy version %d", version)
		}
		evidence.PolicyVersion = version
	}
	evidence.Passed = true
	return evidence, nil
}

type matrixCase struct {
	subject             Subject
	scenario            string
	action              string
	caseName            string
	expectedAllowed     bool
	expectedErrorCode   string
	expectedDenyCode    string
	expectedMatchedRole string
}

func matrixCases(subjects []Subject) []matrixCase {
	byKind := make(map[string]Subject, len(subjects))
	for _, subject := range subjects {
		byKind[subject.Kind] = subject
	}

	result := []matrixCase{}
	for _, kind := range []string{"admin", "operator", "plan_manager", "other"} {
		sub := byKind[kind]
		allowed := kind != "other"
		item := matrixCase{subject: sub, scenario: "retry", action: RetryAction, caseName: "retry", expectedAllowed: allowed}
		if allowed {
			item.expectedMatchedRole = sub.ExpectedRole
		} else {
			item.expectedDenyCode = "policy_not_matched"
		}
		result = append(result, item)
	}
	result = append(result, matrixCase{subject: byKind["operator"], scenario: "force_retry", action: ForceRetryAction, caseName: "force-retry", expectedDenyCode: "policy_not_matched"}, matrixCase{subject: byKind["admin"], scenario: "force_retry", action: ForceRetryAction, caseName: "force-retry", expectedAllowed: true, expectedMatchedRole: RoleAdmin})

	return result
}

func authorizationErrorCode(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, appauthz.ErrAuthorizationContract):
		return "authorization_contract"
	case errors.Is(err, appauthz.ErrAuthorizationUnavailable):
		return "authorization_unavailable"
	default:
		return "unexpected"
	}
}

func validateSubjects(subjects []Subject) error {
	want := map[string]string{"admin": RoleAdmin, "operator": RoleAssessmentOperator, "plan_manager": RolePlanManager, "other": RoleResultReviewer}
	if len(subjects) != len(want) {
		return fmt.Errorf("production subject set has %d entries, want %d", len(subjects), len(want))
	}
	seenIDs := make(map[string]string, len(subjects))
	seenKinds := make(map[string]struct{}, len(subjects))
	for _, subject := range subjects {
		if want[subject.Kind] != subject.ExpectedRole {
			return fmt.Errorf("invalid subject role mapping for %q", subject.Kind)
		}
		parsed, err := strconv.ParseUint(strings.TrimSpace(subject.UserID), 10, 64)
		if err != nil || parsed == 0 || strings.Contains(subject.UserID, ":") {
			return fmt.Errorf("invalid user ID for %q", subject.Kind)
		}
		if prior, exists := seenIDs[subject.UserID]; exists {
			return fmt.Errorf("subject %q is reused by %q and %q", subject.UserID, prior, subject.Kind)
		}
		seenIDs[subject.UserID] = subject.Kind
		seenKinds[subject.Kind] = struct{}{}
	}
	for kind := range want {
		if _, exists := seenKinds[kind]; !exists {
			return fmt.Errorf("missing production subject kind %q", kind)
		}
	}
	return nil
}

func validateResolvedRoles(subject Subject, roles []string) error {
	if !contains(roles, subject.ExpectedRole) {
		return fmt.Errorf("IAM snapshot for %s subject does not contain expected role %s", subject.Kind, subject.ExpectedRole)
	}
	for _, privileged := range []string{RoleAdmin, RoleAssessmentOperator, RolePlanManager} {
		if privileged == subject.ExpectedRole {
			continue
		}
		if subject.Kind != "admin" && contains(roles, privileged) {
			return fmt.Errorf("IAM snapshot for %s subject contains conflicting role %s", subject.Kind, privileged)
		}
	}
	return nil
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func fingerprint(userID string) string {
	sum := sha256.Sum256([]byte("iam-authz-production-subject/v1:" + userID))
	return hex.EncodeToString(sum[:8])
}

func sortedVersions(values map[int64]struct{}) []int64 {
	result := make([]int64, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
