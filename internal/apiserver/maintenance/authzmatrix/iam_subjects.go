package authzmatrix

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	identityv2 "github.com/FangcunMount/iam/v3/api/grpc/iam/identity/v2"
	serviceauth "github.com/FangcunMount/iam/v3/pkg/sdk/auth/serviceauth"
	"github.com/FangcunMount/iam/v3/pkg/sdk/identity"
)

type TokenProvider interface {
	GetToken(context.Context) (string, error)
}

type IAMSyntheticSubjectDirectory struct {
	identity *identity.Client
	tokens   TokenProvider
}

func NewIAMSyntheticSubjectDirectory(identityClient *identity.Client, tokens TokenProvider) *IAMSyntheticSubjectDirectory {
	return &IAMSyntheticSubjectDirectory{identity: identityClient, tokens: tokens}
}

func (d *IAMSyntheticSubjectDirectory) FindActiveIsolatedUser(ctx context.Context, nickname string) (string, error) {
	if d == nil || d.identity == nil || d.tokens == nil {
		return "", fmt.Errorf("IAM identity directory is unavailable")
	}
	authorized, err := authorizedContext(ctx, d.tokens)
	if err != nil {
		return "", err
	}
	resp, err := d.identity.SearchUsers(authorized, &identityv2.SearchUsersRequest{
		Keyword: nickname,
		Page:    &identityv2.OffsetPagination{Limit: 100},
	})
	if err != nil {
		return "", fmt.Errorf("search IAM synthetic subject: %w", err)
	}

	var exact []*identityv2.User
	for _, user := range resp.GetUsers() {
		if user != nil && user.GetNickname() == nickname {
			exact = append(exact, user)
		}
	}
	if len(exact) == 0 {
		return "", fmt.Errorf("%w: IAM synthetic subject %q does not exist", ErrSubjectNotFound, nickname)
	}
	if len(exact) != 1 {
		return "", fmt.Errorf("IAM synthetic subject %q is ambiguous: %d exact users", nickname, len(exact))
	}
	user := exact[0]
	if user.GetStatus() != identityv2.UserStatus_USER_STATUS_ACTIVE {
		return "", fmt.Errorf("IAM synthetic subject %q is not active", nickname)
	}
	if len(user.GetContacts()) != 0 || len(user.GetExternalIdentities()) != 0 {
		return "", fmt.Errorf("IAM synthetic subject %q is not isolated", nickname)
	}
	parsed, err := strconv.ParseUint(strings.TrimSpace(user.GetId()), 10, 64)
	if err != nil || parsed == 0 {
		return "", fmt.Errorf("IAM synthetic subject %q has invalid user ID", nickname)
	}
	return strconv.FormatUint(parsed, 10), nil
}

func authorizedContext(ctx context.Context, tokens TokenProvider) (context.Context, error) {
	if tokens == nil {
		return nil, fmt.Errorf("IAM service token provider is required")
	}
	token, err := tokens.GetToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get IAM service token: %w", err)
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("IAM service token is empty")
	}
	return serviceauth.AuthorizationContext(ctx, token), nil
}
