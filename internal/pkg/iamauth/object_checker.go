package iamauth

import (
	"context"
	"errors"
	"fmt"
	"sort"

	authzv3 "github.com/FangcunMount/iam/v4/api/grpc/iam/authz/v3"
	sdkerrors "github.com/FangcunMount/iam/v4/pkg/sdk/errors"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

type ObjectChecker struct {
	client GRPCClient
}

func NewObjectChecker(client GRPCClient) *ObjectChecker {
	if client == nil || !client.IsEnabled() || client.SDK() == nil {
		return nil
	}
	return &ObjectChecker{client: client}
}

func (c *ObjectChecker) CheckObject(ctx context.Context, request appauthz.ObjectCheckRequest) (appauthz.ObjectDecision, error) {
	if c == nil || c.client == nil || c.client.SDK() == nil {
		return appauthz.ObjectDecision{}, fmt.Errorf("%w: IAM client is not configured", appauthz.ErrAuthorizationContract)
	}
	attributes, err := objectAttributes(request.Attributes)
	if err != nil {
		return appauthz.ObjectDecision{}, fmt.Errorf("%w: %v", appauthz.ErrAuthorizationContract, err)
	}
	response, err := c.client.SDK().Authz().CheckObject(ctx, request.Subject, request.Domain, request.Resource, request.Action, request.ObjectID, attributes)
	if err != nil {
		switch {
		case errors.Is(err, sdkerrors.ErrServiceUnavailable), errors.Is(err, sdkerrors.ErrTimeout):
			return appauthz.ObjectDecision{}, fmt.Errorf("%w: %v", appauthz.ErrAuthorizationUnavailable, err)
		default:
			return appauthz.ObjectDecision{}, fmt.Errorf("%w: %v", appauthz.ErrAuthorizationContract, err)
		}
	}
	return appauthz.ObjectDecision{
		Allowed: response.GetAllowed(), DenyCode: response.GetDenyCode(), PolicyVersion: response.GetPolicyVersion(),
		MatchedGrantID: response.GetMatchedGrantId(), MatchedRole: response.GetMatchedRole(),
		MissingAttributeKeys: append([]string(nil), response.GetMissingAttributeKeys()...),
	}, nil
}

func objectAttributes(input map[string]appauthz.ObjectAttribute) ([]*authzv3.ObjectAttribute, error) {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]*authzv3.ObjectAttribute, 0, len(keys))
	for _, key := range keys {
		value := input[key]
		set := 0
		if value.String != nil {
			set++
		}
		if value.Int64 != nil {
			set++
		}
		if value.Bool != nil {
			set++
		}
		if set != 1 {
			return nil, fmt.Errorf("attribute %s must contain exactly one typed value", key)
		}
		attribute := &authzv3.ObjectAttribute{Key: key}
		switch {
		case value.String != nil:
			attribute.Value = &authzv3.ObjectAttribute_StringValue{StringValue: *value.String}
		case value.Int64 != nil:
			attribute.Value = &authzv3.ObjectAttribute_Int64Value{Int64Value: *value.Int64}
		case value.Bool != nil:
			attribute.Value = &authzv3.ObjectAttribute_BoolValue{BoolValue: *value.Bool}
		}
		result = append(result, attribute)
	}
	return result, nil
}

var _ appauthz.ObjectAuthorizationChecker = (*ObjectChecker)(nil)
