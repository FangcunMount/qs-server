package iam

import iamauth "github.com/FangcunMount/qs-server/internal/pkg/iamauth"

type ActionAuthorizationChecker = iamauth.ActionChecker

func NewActionAuthorizationChecker(client *Client) *ActionAuthorizationChecker {
	return iamauth.NewActionChecker(client)
}
