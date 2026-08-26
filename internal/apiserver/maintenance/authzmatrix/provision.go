package authzmatrix

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	authzv3 "github.com/FangcunMount/iam/v3/api/grpc/iam/authz/v3"
	identityv2 "github.com/FangcunMount/iam/v3/api/grpc/iam/identity/v2"
	"github.com/FangcunMount/iam/v3/pkg/sdk/authz"
	"github.com/FangcunMount/iam/v3/pkg/sdk/identity"
)

const (
	ProvisionConfirmation = "provision-isolated-authz-matrix-evaluator"
	ProvisionActor        = "qs-authz-matrix-provisioner"
)

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

func (p *Provisioner) EnsureEvaluator(ctx context.Context, confirmation string) (ProvisionEvidence, error) {
	if p == nil {
		return ProvisionEvidence{}, fmt.Errorf("IAM provisioner is required")
	}
	evidence := ProvisionEvidence{
		SchemaVersion: "iam-authz-matrix-provision/v1", ProvisionedAt: p.now().UTC(),
		GitCommit: p.gitCommit, ServiceIdentity: p.serviceIdentity,
		Nickname: SyntheticEvaluatorNickname, Role: RoleEvaluator,
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

	userID, err := p.directory.FindActiveIsolatedUser(ctx, SyntheticEvaluatorNickname)
	if errors.Is(err, ErrSubjectNotFound) {
		userID, err = p.createIsolatedUser(ctx)
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
	if hasConflictingProvisionedRole(snapshot.GetRoles()) {
		return evidence, fmt.Errorf("isolated evaluator subject has conflicting roles: %v", sortedStrings(snapshot.GetRoles()))
	}
	if !contains(snapshot.GetRoles(), RoleEvaluator) {
		authorized, authErr := authorizedContext(ctx, p.tokens)
		if authErr != nil {
			return evidence, authErr
		}
		_, authErr = p.authz.GrantAssignment(authorized, &authzv3.GrantAssignmentRequest{
			Subject: "user:" + userID, Domain: Domain, RoleName: RoleEvaluator, GrantedBy: ProvisionActor,
		})
		if authErr != nil {
			return evidence, fmt.Errorf("grant isolated evaluator assignment: %w", authErr)
		}
		evidence.AssignmentCreated = true
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		snapshot, err = p.getSnapshot(ctx, userID)
		if err == nil && contains(snapshot.GetRoles(), RoleEvaluator) && snapshot.GetPolicyVersion() > 0 {
			break
		}
		if time.Now().After(deadline) {
			if err != nil {
				return evidence, fmt.Errorf("observe isolated evaluator assignment: %w", err)
			}
			return evidence, fmt.Errorf("isolated evaluator assignment did not become visible")
		}
		select {
		case <-ctx.Done():
			return evidence, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	if hasConflictingProvisionedRole(snapshot.GetRoles()) {
		return evidence, fmt.Errorf("isolated evaluator subject resolved conflicting roles: %v", sortedStrings(snapshot.GetRoles()))
	}
	evidence.PolicyVersion = snapshot.GetPolicyVersion()
	evidence.Passed = true
	return evidence, nil
}

func (p *Provisioner) createIsolatedUser(ctx context.Context) (string, error) {
	authorized, err := authorizedContext(ctx, p.tokens)
	if err != nil {
		return "", err
	}
	resp, err := p.identity.CreateUser(authorized, &identityv2.CreateUserRequest{
		Nickname: SyntheticEvaluatorNickname,
		Operator: &identityv2.OperatorContext{
			OperatorId: ProvisionActor, OperatorName: ProvisionActor,
			Channel: "production_maintenance", Reason: "stable AuthZ v3 synthetic Check evidence",
		},
	})
	if err != nil {
		return "", fmt.Errorf("create isolated evaluator identity: %w", err)
	}
	user := resp.GetUser()
	if user == nil || user.GetStatus() != identityv2.UserStatus_USER_STATUS_ACTIVE ||
		len(user.GetContacts()) != 0 || len(user.GetExternalIdentities()) != 0 {
		return "", fmt.Errorf("created evaluator identity is not active and isolated")
	}
	return p.directory.FindActiveIsolatedUser(ctx, SyntheticEvaluatorNickname)
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
		return nil, fmt.Errorf("load isolated evaluator snapshot: %w", err)
	}
	return resp, nil
}

func hasConflictingProvisionedRole(roles []string) bool {
	for _, role := range roles {
		switch role {
		case RoleAdmin, RolePlanManager:
			return true
		}
	}
	return false
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
