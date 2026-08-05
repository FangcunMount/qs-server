package iam

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ResolveUserNames fetches user nicknames by ID through IAM.
// It returns a map of user_id -> nickname; missing entries should fall back to ID string.
func ResolveUserNames(ctx context.Context, identitySvc *IdentityService, ids []meta.ID) map[string]string {
	if identitySvc == nil || !identitySvc.IsEnabled() {
		return nil
	}

	userIDs := make(map[string]struct{})
	for _, id := range ids {
		if id.IsZero() {
			continue
		}
		userIDs[id.String()] = struct{}{}
	}

	if len(userIDs) == 0 {
		return nil
	}

	uniqueIDs := make([]string, 0, len(userIDs))
	for id := range userIDs {
		uniqueIDs = append(uniqueIDs, id)
	}

	resp, err := identitySvc.BatchGetUsers(ctx, uniqueIDs)
	if err != nil {
		return nil
	}

	userNames := make(map[string]string)
	for _, user := range resp.GetUsers() {
		if user == nil {
			continue
		}
		if name := user.GetNickname(); name != "" {
			userNames[user.GetId()] = name
		}
	}

	return userNames
}
