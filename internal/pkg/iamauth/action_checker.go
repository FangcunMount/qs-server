package iamauth

import (
	"context"
	"errors"
	"fmt"

	authzv4 "github.com/FangcunMount/iam/v5/api/grpc/iam/authz/v4"
	sdkerrors "github.com/FangcunMount/iam/v5/pkg/sdk/errors"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

type ActionChecker struct {
	client GRPCClient
}

func NewActionChecker(client GRPCClient) *ActionChecker {
	if client == nil || !client.IsEnabled() || client.SDK() == nil {
		return nil
	}
	return &ActionChecker{client: client}
}

func (c *ActionChecker) CheckAction(ctx context.Context, request appauthz.ActionCheckRequest) (appauthz.ActionDecision, error) {
	if c == nil || c.client == nil || c.client.SDK() == nil {
		return appauthz.ActionDecision{}, fmt.Errorf("%w: IAM client is not configured", appauthz.ErrAuthorizationContract)
	}
	response, err := c.client.SDK().Authz().Check(ctx, &authzv4.CheckRequest{Subject: request.Subject, Resource: request.Resource, Action: request.Action})
	if err != nil {
		switch {
		case errors.Is(err, sdkerrors.ErrServiceUnavailable), errors.Is(err, sdkerrors.ErrTimeout):
			return appauthz.ActionDecision{}, fmt.Errorf("%w: %v", appauthz.ErrAuthorizationUnavailable, err)
		default:
			return appauthz.ActionDecision{}, fmt.Errorf("%w: %v", appauthz.ErrAuthorizationContract, err)
		}
	}
	return appauthz.ActionDecision{
		Allowed: response.GetAllowed(), DenyCode: response.GetDenyCode(), PolicyVersion: response.GetPolicyVersion(),
		MatchedGrantID: response.GetMatchedGrantId(), MatchedRole: response.GetMatchedRole(),
	}, nil
}
