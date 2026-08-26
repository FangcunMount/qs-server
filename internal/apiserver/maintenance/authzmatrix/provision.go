package authzmatrix

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	authzv3 "github.com/FangcunMount/iam/v3/api/grpc/iam/authz/v3"
	identityv2 "github.com/FangcunMount/iam/v3/api/grpc/iam/identity/v2"
	"github.com/FangcunMount/iam/v3/pkg/sdk/authz"
	"github.com/FangcunMount/iam/v3/pkg/sdk/identity"
)

const (
	ProvisionConfirmation = "provision-isolated-authz-matrix-subjects-v2"
	ProvisionActor        = "qs-authz-matrix-provisioner"
)

type ProvisionBatchEvidence struct {
	SchemaVersion   string              `json:"schema_version"`
	ProvisionedAt   time.Time           `json:"provisioned_at"`
	GitCommit       string              `json:"git_commit"`
	ServiceIdentity string              `json:"service_identity"`
	Subjects        []ProvisionEvidence `json:"subjects"`
	Passed          bool                `json:"passed"`
}

type ProvisionEvidence struct {
	SchemaVersion      string    `json:"schema_version"`
	ProvisionedAt      time.Time `json:"provisioned_at"`
	GitCommit          string    `json:"git_commit"`
	ServiceIdentity    string    `json:"service_identity"`
	Nickname           string    `json:"nickname"`
	SubjectFingerprint string    `json:"subject_fingerprint"`
	Role               string    `json:"role"`
	UserCreated        bool      `json:"user_created"`
	AssignmentCreated  bool      `json:"assignment_created"`
	PolicyVersion      int64     `json:"policy_version"`
	Passed             bool      `json:"passed"`
}

type Provisioner struct {
	identity        *identity.Client
	authz           *authz.Client
	directory       *IAMSyntheticSubjectDirectory
	tokens          TokenProvider
	gitCommit       string
	serviceIdentity string
	now             func() time.Time
}

func NewProvisioner(identityClient *identity.Client, authzClient *authz.Client, tokens TokenProvider, gitCommit, serviceIdentity string) *Provisioner {
	return &Provisioner{
		identity: identityClient, authz: authzClient,
		directory: NewIAMSyntheticSubjectDirectory(identityClient, tokens), tokens: tokens,
		gitCommit: strings.TrimSpace(gitCommit), serviceIdentity: strings.TrimSpace(serviceIdentity), now: time.Now,
	}
}

func (p *Provisioner) EnsureSubjects(ctx context.Context, confirmation string) (ProvisionBatchEvidence, error) {
	if p == nil {
		return ProvisionBatchEvidence{}, fmt.Errorf("IAM provisioner is required")
	}
	evidence := ProvisionBatchEvidence{
		SchemaVersion: "iam-authz-matrix-provision/v2", ProvisionedAt: p.now().UTC(),
		GitCommit: p.gitCommit, ServiceIdentity: p.serviceIdentity,
	}
	if confirmation != ProvisionConfirmation {
		return evidence, fmt.Errorf("explicit provisioning confirmation is required")
	}
	if p.identity == nil || p.authz == nil || p.directory == nil || p.tokens == nil {
		return evidence, fmt.Errorf("IAM provisioning dependencies are unavailable")
	}
	if p.serviceIdentity != "qs-apiserver.svc" {
		return evidence, fmt.Errorf("unexpected service identity %q", p.serviceIdentity)
	}
	for _, subject := range []struct {
		nickname string
		role     string
	}{
		{nickname: SyntheticEvaluatorNickname, role: RoleEvaluator},
		{nickname: SyntheticPlanManagerNickname, role: RolePlanManager},
	} {
		subjectEvidence, err := p.ensureSubject(ctx, subject.nickname, subject.role)
		evidence.Subjects = append(evidence.Subjects, subjectEvidence)
		if err != nil {
			return evidence, err
		}
	}
	evidence.Passed = true
	return evidence, nil
}

func (p *Provisioner) ensureSubject(ctx context.Context, nickname, role string) (ProvisionEvidence, error) {
	evidence := ProvisionEvidence{
		SchemaVersion: "iam-authz-matrix-subject/v2", ProvisionedAt: p.now().UTC(),
		GitCommit: p.gitCommit, ServiceIdentity: p.serviceIdentity, Nickname: nickname, Role: role,
	}

	userID, err := p.directory.FindActiveIsolatedUser(ctx, nickname)
	if errors.Is(err, ErrSubjectNotFound) {
		userID, err = p.createIsolatedUser(ctx, nickname)
		evidence.UserCreated = err == nil
	}
	if err != nil {
		return evidence, err
	}
	evidence.SubjectFingerprint = fingerprint(userID)

	snapshot, err := p.getSnapshot(ctx, userID)
	if err != nil {
		return evidence, err
	}
	if !equalRoles(snapshot.GetDirectRoles(), []string{role}) {
		authorized, authErr := authorizedContext(ctx, p.tokens)
		if authErr != nil {
			return evidence, authErr
		}
		resp, authErr := p.authz.ReplaceManagedAssignments(authorized, &authzv3.ReplaceManagedAssignmentsRequest{
			Subject: "user:" + userID, Domain: Domain, RoleNames: []string{role}, ChangedBy: ProvisionActor,
			Reason: "stable AuthZ v3 synthetic Check evidence",
		})
		if authErr != nil {
			return evidence, fmt.Errorf("replace isolated %s assignments: %w", role, authErr)
		}
		evidence.AssignmentCreated = resp.GetChanged()
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		snapshot, err = p.getSnapshot(ctx, userID)
		if err == nil && equalRoles(snapshot.GetDirectRoles(), []string{role}) && snapshot.GetPolicyVersion() > 0 {
			break
		}
		if time.Now().After(deadline) {
			if err != nil {
				return evidence, fmt.Errorf("observe isolated %s assignment: %w", role, err)
			}
			return evidence, fmt.Errorf("isolated %s assignment did not become visible", role)
		}
		select {
		case <-ctx.Done():
			return evidence, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	evidence.PolicyVersion = snapshot.GetPolicyVersion()
	evidence.Passed = true
	return evidence, nil
}

func (p *Provisioner) createIsolatedUser(ctx context.Context, nickname string) (string, error) {
	authorized, err := authorizedContext(ctx, p.tokens)
	if err != nil {
		return "", err
	}
	resp, err := p.identity.CreateUser(authorized, &identityv2.CreateUserRequest{
		Nickname: nickname,
		Operator: &identityv2.OperatorContext{
			OperatorId: ProvisionActor, OperatorName: ProvisionActor,
			Channel: "production_maintenance", Reason: "stable AuthZ v3 synthetic Check evidence",
		},
	})
	if err != nil {
		return "", fmt.Errorf("create isolated matrix identity %q: %w", nickname, err)
	}
	return createdIsolatedUserID(resp.GetUser(), nickname)
}

func createdIsolatedUserID(user *identityv2.User, nickname string) (string, error) {
	if user == nil || user.GetNickname() != nickname || user.GetStatus() != identityv2.UserStatus_USER_STATUS_ACTIVE ||
		len(user.GetContacts()) != 0 || len(user.GetExternalIdentities()) != 0 {
		return "", fmt.Errorf("created matrix identity %q is not active and isolated", nickname)
	}
	parsed, err := strconv.ParseUint(strings.TrimSpace(user.GetId()), 10, 64)
	if err != nil || parsed == 0 {
		return "", fmt.Errorf("created matrix identity %q has invalid user ID", nickname)
	}
	return strconv.FormatUint(parsed, 10), nil
}

func (p *Provisioner) getSnapshot(ctx context.Context, userID string) (*authzv3.GetAuthorizationSnapshotResponse, error) {
	authorized, err := authorizedContext(ctx, p.tokens)
	if err != nil {
		return nil, err
	}
	resp, err := p.authz.GetAuthorizationSnapshot(authorized, &authzv3.GetAuthorizationSnapshotRequest{
		Subject: "user:" + userID, Domain: Domain, AppName: "qs",
	})
	if err != nil {
		return nil, fmt.Errorf("load isolated matrix snapshot: %w", err)
	}
	return resp, nil
}

func equalRoles(got, want []string) bool {
	return strings.Join(sortedStrings(got), "\x00") == strings.Join(sortedStrings(want), "\x00")
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
