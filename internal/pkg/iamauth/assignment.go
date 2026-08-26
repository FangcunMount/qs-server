package iamauth

import (
	"context"
	"fmt"

	authzv3 "github.com/FangcunMount/iam/v3/api/grpc/iam/authz/v3"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

// AssignmentClient IAM GrantAssignment / RevokeAssignment。
type AssignmentClient struct {
	client GRPCClient
	tokens TokenProvider
}

type ReplaceAssignmentsResult struct {
	DirectRoles   []string
	PolicyVersion int64
	Changed       bool
}

// NewAssignmentClient 创建客户端；IAM 未启用时返回 nil。
func NewAssignmentClient(c GRPCClient, providers ...TokenProvider) *AssignmentClient {
	if c == nil || !c.IsEnabled() || c.SDK() == nil {
		return nil
	}
	var tokens TokenProvider
	if len(providers) > 0 {
		tokens = providers[0]
	}
	return &AssignmentClient{client: c, tokens: tokens}
}

// Grant 授予 IAM 角色。
func (a *AssignmentClient) Grant(ctx context.Context, domain, targetUserIDStr, roleName, grantedBy string) error {
	if a == nil || a.client == nil {
		return fmt.Errorf("iam assignment client not available")
	}
	ctx, err := authorizationContext(ctx, a.tokens)
	if err != nil {
		return err
	}
	_, err = a.client.SDK().Authz().GrantAssignment(ctx, &authzv3.GrantAssignmentRequest{
		Subject:   authz.SubjectKey(targetUserIDStr),
		Domain:    domain,
		RoleName:  roleName,
		GrantedBy: grantedBy,
	})
	return err
}

// Revoke 撤销 IAM 角色。
func (a *AssignmentClient) Revoke(ctx context.Context, domain, targetUserIDStr, roleName string) error {
	if a == nil || a.client == nil {
		return fmt.Errorf("iam assignment client not available")
	}
	ctx, err := authorizationContext(ctx, a.tokens)
	if err != nil {
		return err
	}
	_, err = a.client.SDK().Authz().RevokeAssignment(ctx, &authzv3.RevokeAssignmentRequest{
		Subject:  authz.SubjectKey(targetUserIDStr),
		Domain:   domain,
		RoleName: roleName,
	})
	return err
}

// ReplaceManaged atomically replaces only the QS roles delegated to qs-apiserver.
func (a *AssignmentClient) ReplaceManaged(ctx context.Context, domain, targetUserIDStr string, roleNames []string, changedBy, reason string) (*ReplaceAssignmentsResult, error) {
	if a == nil || a.client == nil {
		return nil, fmt.Errorf("iam assignment client not available")
	}
	ctx, err := authorizationContext(ctx, a.tokens)
	if err != nil {
		return nil, err
	}
	resp, err := a.client.SDK().Authz().ReplaceManagedAssignments(ctx, &authzv3.ReplaceManagedAssignmentsRequest{
		Subject: authz.SubjectKey(targetUserIDStr), Domain: domain, RoleNames: append([]string(nil), roleNames...),
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
