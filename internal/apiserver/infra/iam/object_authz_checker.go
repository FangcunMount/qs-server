package iam

import iamauth "github.com/FangcunMount/qs-server/internal/pkg/iamauth"

type ObjectAuthorizationChecker = iamauth.ObjectChecker

func NewObjectAuthorizationChecker(client *Client) *ObjectAuthorizationChecker {
	return iamauth.NewObjectChecker(client)
}
