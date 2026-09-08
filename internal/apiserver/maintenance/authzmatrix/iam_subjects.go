package authzmatrix

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	identityv2 "github.com/FangcunMount/iam/v5/api/grpc/iam/identity/v2"
	"github.com/FangcunMount/iam/v5/pkg/sdk/identity"
)

type IAMSyntheticSubjectDirectory struct {
	identity *identity.Client
}

func NewIAMSyntheticSubjectDirectory(identityClient *identity.Client) *IAMSyntheticSubjectDirectory {
	return &IAMSyntheticSubjectDirectory{identity: identityClient}
}

func (d *IAMSyntheticSubjectDirectory) FindActiveIsolatedUser(ctx context.Context, nickname string) (string, error) {
	userIDs, err := d.FindActiveIsolatedUsers(ctx, nickname)
	if err != nil {
		return "", err
	}
	if len(userIDs) != 1 {
		return "", fmt.Errorf("IAM synthetic subject %q is ambiguous: %d exact active isolated users", nickname, len(userIDs))
	}
	return userIDs[0], nil
}

func (d *IAMSyntheticSubjectDirectory) FindActiveIsolatedUsers(ctx context.Context, nickname string) ([]string, error) {
	if d == nil || d.identity == nil {
		return nil, fmt.Errorf("IAM identity directory is unavailable")
	}
	resp, err := d.identity.SearchUsers(ctx, &identityv2.SearchUsersRequest{
		Keyword: nickname,
		Page:    &identityv2.OffsetPagination{Limit: 100},
	})
	if err != nil {
		return nil, fmt.Errorf("search IAM synthetic subject: %w", err)
	}

	var userIDs []string
	for _, user := range resp.GetUsers() {
		if user == nil || user.GetNickname() != nickname || user.GetStatus() != identityv2.UserStatus_USER_STATUS_ACTIVE ||
			len(user.GetContacts()) != 0 || len(user.GetExternalIdentities()) != 0 {
			continue
		}
		parsed, parseErr := strconv.ParseUint(strings.TrimSpace(user.GetId()), 10, 64)
		if parseErr != nil || parsed == 0 {
			return nil, fmt.Errorf("IAM synthetic subject %q has invalid user ID", nickname)
		}
		userIDs = append(userIDs, strconv.FormatUint(parsed, 10))
	}
	if len(userIDs) == 0 {
		return nil, fmt.Errorf("%w: IAM synthetic subject %q does not exist", ErrSubjectNotFound, nickname)
	}
	return userIDs, nil
}
