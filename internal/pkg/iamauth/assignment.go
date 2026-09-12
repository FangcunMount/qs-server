package iamauth

import (
	"context"
	"fmt"

	authzv4 "github.com/FangcunMount/iam/v5/api/grpc/iam/authz/v4"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

// AssignmentClient atomically replaces the QS-managed direct role set.
type AssignmentClient struct {
	client GRPCClient
}

type ReplaceAssignmentsResult struct {
	DirectRoles   []string
	PolicyVersion int64
	Changed       bool
}

// NewAssignmentClient 创建客户端；IAM 未启用时返回 nil。
func NewAssignmentClient(c GRPCClient) *AssignmentClient {
	if c == nil || !c.IsEnabled() || c.SDK() == nil {
		return nil
	}
	return &AssignmentClient{client: c}
}

// ReplaceManaged atomically replaces only the QS roles delegated to qs-apiserver.
func (a *AssignmentClient) ReplaceManaged(ctx context.Context, targetUserIDStr string, roleNames []string, changedBy, reason string) (*ReplaceAssignmentsResult, error) {
	if a == nil || a.client == nil {
		return nil, fmt.Errorf("iam assignment client not available")
	}
	resp, err := a.client.SDK().Authz().ReplaceManagedAssignments(ctx, &authzv4.ReplaceManagedAssignmentsRequest{
		Subject: authz.SubjectKey(targetUserIDStr), RoleNames: append([]string(nil), roleNames...),
		ChangedBy: changedBy, Reason: reason,
	})
	if err != nil {
		return nil, err
	}
	return &ReplaceAssignmentsResult{
		DirectRoles:   append([]string(nil), resp.GetDirectRoles()...),
		PolicyVersion: resp.GetPolicyVersion(), Changed: resp.GetChanged(),
	}, nil
}

// ReplaceScoped uses the company-specific RPC; never falls back to legacy
// replacement, which has no company boundary.
func (a *AssignmentClient) ReplaceScoped(ctx context.Context, userID, orgID string, roles []*authzv4.ScopedRoleAssignment, expectedVersion int64, changedBy, reason string) (*ReplaceAssignmentsResult, error) {
	if expectedVersion <= 0 {
		return nil, fmt.Errorf("positive expected policy version required")
	}
	if a == nil || a.client == nil {
		return nil, fmt.Errorf("iam assignment client not available")
	}
	resp, err := a.client.SDK().Authz().ReplaceScopedAssignments(ctx, &authzv4.ReplaceScopedAssignmentsRequest{ExpectedPolicyVersion: expectedVersion, Subject: authz.SubjectKey(userID), OrgId: orgID, Roles: roles, ChangedBy: changedBy, Reason: reason})
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.PolicyVersion <= 0 {
		return nil, fmt.Errorf("invalid scoped replacement result")
	}
	return &ReplaceAssignmentsResult{DirectRoles: append([]string(nil), resp.DirectRoles...), PolicyVersion: resp.PolicyVersion, Changed: resp.Changed}, nil
}
