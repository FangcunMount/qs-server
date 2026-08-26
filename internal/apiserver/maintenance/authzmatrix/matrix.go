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
	Domain             = "fangcun"
	AssessmentResource = "qs:evaluation:collection:assessments"
	RetryAction        = "retry"

	RoleAdmin       = "qs:admin"
	RoleEvaluator   = "qs:evaluator"
	RolePlanManager = "qs:evaluation_plan_manager"
	RoleStaff       = "qs:staff"
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
	Load(context.Context, string, string) (*appauthz.Snapshot, error)
}

type ObjectChecker interface {
	CheckObject(context.Context, appauthz.ObjectCheckRequest) (appauthz.ObjectDecision, error)
}

type Evidence struct {
	SchemaVersion   string            `json:"schema_version"`
	CheckedAt       time.Time         `json:"checked_at"`
	GitCommit       string            `json:"git_commit"`
	ServiceIdentity string            `json:"service_identity"`
	Domain          string            `json:"domain"`
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
	Kind                 string   `json:"kind"`
	OriginType           string   `json:"origin_type,omitempty"`
	ExpectedAllowed      bool     `json:"expected_allowed"`
	Allowed              bool     `json:"allowed"`
	DenyCode             string   `json:"deny_code,omitempty"`
	MatchedRole          string   `json:"matched_role,omitempty"`
	MatchedGrantID       string   `json:"matched_grant_id,omitempty"`
	MissingAttributeKeys []string `json:"missing_attribute_keys,omitempty"`
	PolicyVersion        int64    `json:"policy_version"`
	Passed               bool     `json:"passed"`
}

type Runner struct {
	subjects        SubjectSource
	snapshots       SnapshotReader
	checker         ObjectChecker
	now             func() time.Time
	gitCommit       string
	serviceIdentity string
}

func NewRunner(subjects SubjectSource, snapshots SnapshotReader, checker ObjectChecker, gitCommit, serviceIdentity string) *Runner {
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
		SchemaVersion: "iam-authz-production-matrix/v1",
		CheckedAt:     r.now().UTC(), GitCommit: r.gitCommit, ServiceIdentity: r.serviceIdentity,
		Domain: Domain, Resource: AssessmentResource, Action: RetryAction,
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
		snapshot, err := r.snapshots.Load(ctx, Domain, subject.UserID)
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

	for _, testCase := range matrixCases(subjects) {
		request := appauthz.ObjectCheckRequest{
			Subject: appauthz.SubjectKey(testCase.subject.UserID), Domain: Domain,
			Resource: AssessmentResource, Action: RetryAction,
			ObjectID:   "authz-production-matrix:" + testCase.objectSuffix,
			Attributes: map[string]appauthz.ObjectAttribute{},
		}
		if testCase.originType != "" {
			request.Attributes[appauthz.ObjectOriginTypeAttribute] = appauthz.StringAttribute(testCase.originType)
		}
		decision, err := r.checker.CheckObject(ctx, request)
		if err != nil {
			return evidence, fmt.Errorf("check %s/%s: %w", testCase.subject.Kind, testCase.objectSuffix, err)
		}
		versions[decision.PolicyVersion] = struct{}{}
		passed := decision.Allowed == testCase.expectedAllowed
		if testCase.expectedDenyCode != "" {
			passed = passed && decision.DenyCode == testCase.expectedDenyCode
		}
		if testCase.expectedMatchedRole != "" {
			passed = passed && decision.MatchedRole == testCase.expectedMatchedRole
		}
		missing := append([]string(nil), decision.MissingAttributeKeys...)
		sort.Strings(missing)
		if testCase.expectMissingOrigin {
			passed = passed && contains(missing, appauthz.ObjectOriginTypeAttribute)
		}
		evidence.Cases = append(evidence.Cases, CaseEvidence{
			Kind: testCase.subject.Kind, OriginType: testCase.originType,
			ExpectedAllowed: testCase.expectedAllowed, Allowed: decision.Allowed,
			DenyCode: decision.DenyCode, MatchedRole: decision.MatchedRole,
			MatchedGrantID: decision.MatchedGrantID, MissingAttributeKeys: missing,
			PolicyVersion: decision.PolicyVersion, Passed: passed,
		})
		if !passed {
			return evidence, fmt.Errorf("authorization matrix mismatch for %s/%s", testCase.subject.Kind, testCase.objectSuffix)
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
	originType          string
	objectSuffix        string
	expectedAllowed     bool
	expectedDenyCode    string
	expectedMatchedRole string
	expectMissingOrigin bool
}

func matrixCases(subjects []Subject) []matrixCase {
	byKind := make(map[string]Subject, len(subjects))
	for _, subject := range subjects {
		byKind[subject.Kind] = subject
	}
	result := make([]matrixCase, 0, 12)
	for _, origin := range []string{"adhoc", "plan"} {
		for _, kind := range []string{"admin", "evaluator", "plan_manager", "other"} {
			subject := byKind[kind]
			allowed := kind == "admin" || (kind == "evaluator" && origin == "adhoc") || (kind == "plan_manager" && origin == "plan")
			item := matrixCase{subject: subject, originType: origin, objectSuffix: origin, expectedAllowed: allowed}
			if allowed {
				item.expectedMatchedRole = subject.ExpectedRole
			} else {
				item.expectedDenyCode = "policy_not_matched"
			}
			result = append(result, item)
		}
	}
	for _, kind := range []string{"admin", "evaluator", "plan_manager", "other"} {
		subject := byKind[kind]
		item := matrixCase{subject: subject, objectSuffix: "attribute-missing"}
		switch kind {
		case "admin":
			item.expectedAllowed = true
			item.expectedMatchedRole = RoleAdmin
		case "evaluator", "plan_manager":
			item.expectedDenyCode = "attribute_missing"
			item.expectMissingOrigin = true
		default:
			item.expectedDenyCode = "policy_not_matched"
		}
		result = append(result, item)
	}
	return result
}

func validateSubjects(subjects []Subject) error {
	want := map[string]string{"admin": RoleAdmin, "evaluator": RoleEvaluator, "plan_manager": RolePlanManager, "other": RoleStaff}
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
	for _, privileged := range []string{RoleAdmin, RoleEvaluator, RolePlanManager} {
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
