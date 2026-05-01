package iamauth

import (
	"context"
	"fmt"

	authzv2 "github.com/FangcunMount/iam/v2/api/grpc/iam/authz/v2"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

// AssignmentClient IAM GrantAssignment / RevokeAssignment。
type AssignmentClient struct {
	client GRPCClient
}

// NewAssignmentClient 创建客户端；IAM 未启用时返回 nil。
func NewAssignmentClient(c GRPCClient) *AssignmentClient {
	if c == nil || !c.IsEnabled() || c.SDK() == nil {
		return nil
	}
	return &AssignmentClient{client: c}
}

// Grant 授予 IAM 角色。
func (a *AssignmentClient) Grant(ctx context.Context, domain, targetUserIDStr, roleName, grantedBy string) error {
	if a == nil || a.client == nil {
		return fmt.Errorf("iam assignment client not available")
	}
	_, err := a.client.SDK().Authz().GrantAssignment(ctx, &authzv2.GrantAssignmentRequest{
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
	_, err := a.client.SDK().Authz().RevokeAssignment(ctx, &authzv2.RevokeAssignmentRequest{
		Subject:  authz.SubjectKey(targetUserIDStr),
		Domain:   domain,
		RoleName: roleName,
	})
	return err
}
