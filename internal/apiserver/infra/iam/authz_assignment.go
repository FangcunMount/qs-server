package iam

import iamauth "github.com/FangcunMount/qs-server/internal/pkg/iamauth"

// AuthzAssignmentClient IAM Assignment 门面（见 pkg/iamauth）。
type AuthzAssignmentClient = iamauth.AssignmentClient

// NewAuthzAssignmentClient 创建客户端。
func NewAuthzAssignmentClient(c *Client) *AuthzAssignmentClient {
	return iamauth.NewAssignmentClient(c)
}
