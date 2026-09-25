package iam

import (
	"context"
	"fmt"
	"slices"
	"strings"

	authnv3 "github.com/FangcunMount/iam/v5/api/grpc/iam/authn/v3"
	identityv2 "github.com/FangcunMount/iam/v5/api/grpc/iam/identity/v2"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
)

const maxMiniProgramRecipientUsers = 20

type notificationProfileLinks interface {
	IsEnabled() bool
	ListProfileLinks(context.Context, string) (*identityv2.ListProfileLinksResponse, error)
}

type notificationIdentityLookup interface {
	ResolveMiniProgramRecipients(context.Context, *authnv3.ResolveMiniProgramNotificationRecipientsRequest) (*authnv3.ResolveMiniProgramNotificationRecipientsResponse, error)
}

type miniProgramRecipientCandidateReader struct {
	links  notificationProfileLinks
	lookup notificationIdentityLookup
}

// NewMiniProgramRecipientCandidateReader prepares the IAM-backed candidate
// reader. It is intentionally not wired to the legacy task.opened sender.
func NewMiniProgramRecipientCandidateReader(
	links *ProfileLinkService,
	client *Client,
) iambridge.MiniProgramRecipientCandidateReader {
	if links == nil || !links.IsEnabled() || client == nil || !client.IsEnabled() || client.SDK() == nil || client.SDK().Auth() == nil {
		return nil
	}
	return &miniProgramRecipientCandidateReader{links: links, lookup: client.SDK().Auth()}
}

func (r *miniProgramRecipientCandidateReader) ReadCandidates(
	ctx context.Context,
	profileID, appID string,
	selectedUserIDs []string,
) ([]iambridge.MiniProgramRecipientCandidate, error) {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return nil, fmt.Errorf("mini-program AppID is required")
	}
	if r == nil || r.lookup == nil {
		return nil, fmt.Errorf("mini-program recipient identity lookup is unavailable")
	}
	relations, err := r.readActiveLinkRelations(ctx, profileID)
	if err != nil {
		return nil, err
	}
	if len(selectedUserIDs) == 0 {
		return []iambridge.MiniProgramRecipientCandidate{}, nil
	}
	selected := make(map[string]map[string]struct{}, len(selectedUserIDs))
	for _, rawUserID := range selectedUserIDs {
		userID := strings.TrimSpace(rawUserID)
		userRelations, linkedUser := relations[userID]
		if !linkedUser || userID == "" {
			return nil, fmt.Errorf("selected notification user has no active link to the profile")
		}
		selected[userID] = userRelations
	}
	if len(selected) > maxMiniProgramRecipientUsers {
		return nil, fmt.Errorf("selected users exceed IAM recipient lookup limit")
	}
	userIDs := make([]string, 0, len(selected))
	for userID := range selected {
		userIDs = append(userIDs, userID)
	}
	slices.Sort(userIDs)
	resp, err := r.lookup.ResolveMiniProgramRecipients(ctx, &authnv3.ResolveMiniProgramNotificationRecipientsRequest{
		UserIds: userIDs,
		AppId:   appID,
	})
	if err != nil {
		return nil, fmt.Errorf("read mini-program recipient identities: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("IAM returned no recipient lookup response")
	}

	candidates := make([]iambridge.MiniProgramRecipientCandidate, 0, len(resp.GetItems()))
	seen := make(map[string]iambridge.MiniProgramRecipientCandidate)
	for _, item := range resp.GetItems() {
		if item == nil {
			return nil, fmt.Errorf("IAM returned an empty recipient identity")
		}
		userRelations, linkedUser := selected[item.GetUserId()]
		if !linkedUser || item.GetAppId() != appID || item.GetLoginIdentityId() == "" || item.GetOpenId() == "" {
			return nil, fmt.Errorf("IAM returned a recipient identity outside the requested profile and AppID")
		}
		key := item.GetLoginIdentityId()
		if previous, exists := seen[key]; exists {
			if previous.UserID != item.GetUserId() || previous.AppID != item.GetAppId() || previous.OpenID != item.GetOpenId() {
				return nil, fmt.Errorf("IAM returned conflicting recipient identities")
			}
			continue
		}
		candidate := iambridge.MiniProgramRecipientCandidate{
			UserID:          item.GetUserId(),
			LoginIdentityID: key,
			AppID:           appID,
			OpenID:          item.GetOpenId(),
			Relations:       make([]string, 0, len(userRelations)),
		}
		for relation := range userRelations {
			candidate.Relations = append(candidate.Relations, relation)
		}
		slices.Sort(candidate.Relations)
		seen[key] = candidate
		candidates = append(candidates, candidate)
	}
	slices.SortFunc(candidates, func(a, b iambridge.MiniProgramRecipientCandidate) int {
		if cmp := strings.Compare(a.UserID, b.UserID); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.LoginIdentityID, b.LoginIdentityID)
	})
	return candidates, nil
}

func (r *miniProgramRecipientCandidateReader) ListLinkedUsers(
	ctx context.Context,
	profileID string,
) ([]iambridge.MiniProgramLinkedUser, error) {
	relations, err := r.readActiveLinkRelations(ctx, profileID)
	if err != nil {
		return nil, err
	}
	users := make([]iambridge.MiniProgramLinkedUser, 0, len(relations))
	for userID, userRelations := range relations {
		user := iambridge.MiniProgramLinkedUser{UserID: userID, Relations: make([]string, 0, len(userRelations))}
		for relation := range userRelations {
			user.Relations = append(user.Relations, relation)
		}
		slices.Sort(user.Relations)
		users = append(users, user)
	}
	slices.SortFunc(users, func(a, b iambridge.MiniProgramLinkedUser) int {
		return strings.Compare(a.UserID, b.UserID)
	})
	return users, nil
}

func (r *miniProgramRecipientCandidateReader) readActiveLinkRelations(
	ctx context.Context,
	profileID string,
) (map[string]map[string]struct{}, error) {
	profileID = strings.TrimSpace(profileID)
	if r == nil || r.links == nil || !r.links.IsEnabled() {
		return nil, fmt.Errorf("mini-program profile link reader is unavailable")
	}
	if profileID == "" {
		return nil, fmt.Errorf("profile ID is required")
	}
	linked, err := r.links.ListProfileLinks(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("read profile links: %w", err)
	}
	relations := make(map[string]map[string]struct{})
	if linked != nil {
		for _, edge := range linked.GetItems() {
			if edge == nil || edge.ProfileLink == nil || edge.ProfileLink.GetRevokedAt() != nil {
				continue
			}
			link := edge.ProfileLink
			if link.GetProfileId() != "" && link.GetProfileId() != profileID {
				return nil, fmt.Errorf("IAM returned a link for another profile")
			}
			userID := strings.TrimSpace(link.GetUserId())
			if userID == "" {
				continue
			}
			if relations[userID] == nil {
				relations[userID] = make(map[string]struct{})
			}
			relations[userID][miniProgramLinkRelation(link.GetRelation())] = struct{}{}
		}
	}
	return relations, nil
}

func miniProgramLinkRelation(relation identityv2.ProfileLinkRelation) string {
	switch relation {
	case identityv2.ProfileLinkRelation_PROFILE_LINK_RELATION_SELF:
		return "self"
	case identityv2.ProfileLinkRelation_PROFILE_LINK_RELATION_PARENT:
		return "parent"
	case identityv2.ProfileLinkRelation_PROFILE_LINK_RELATION_GRANDPARENT:
		return "grandparent"
	case identityv2.ProfileLinkRelation_PROFILE_LINK_RELATION_OTHER:
		return "other"
	default:
		return "unknown"
	}
}
