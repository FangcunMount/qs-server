package iam

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	identityv2 "github.com/FangcunMount/iam/v2/api/grpc/iam/identity/v2"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func (s *WeChatAppService) ResolveWeChatAppConfig(ctx context.Context, appID string) (*iambridge.WeChatAppConfig, error) {
	resp, err := s.GetWechatApp(ctx, appID)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.App == nil {
		return nil, fmt.Errorf("IAM returned empty wechat app")
	}
	return &iambridge.WeChatAppConfig{
		AppID:     resp.App.GetAppId(),
		AppSecret: resp.App.GetAppSecret(),
	}, nil
}

func (s *IdentityService) ResolveUserNames(ctx context.Context, ids []meta.ID) map[string]string {
	return ResolveUserNames(ctx, s, ids)
}

type userDirectory struct {
	svc *IdentityService
}

func NewUserDirectory(svc *IdentityService) iambridge.UserDirectory {
	if svc == nil {
		return nil
	}
	return &userDirectory{svc: svc}
}

func (d *userDirectory) IsEnabled() bool {
	return d != nil && d.svc != nil && d.svc.IsEnabled()
}

func (d *userDirectory) FindUserIDByPhone(ctx context.Context, phone string) (int64, bool, error) {
	if !d.IsEnabled() || strings.TrimSpace(phone) == "" {
		return 0, false, nil
	}
	resp, err := d.svc.SearchUsers(ctx, &identityv2.SearchUsersRequest{Phones: []string{phone}})
	if err != nil {
		return 0, false, err
	}
	if resp == nil || len(resp.Users) == 0 {
		return 0, false, nil
	}
	uidStr := strings.TrimSpace(resp.Users[0].GetId())
	if uidStr == "" {
		return 0, false, nil
	}
	uid, err := strconv.ParseInt(uidStr, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("failed to parse user id from IAM search result: %w", err)
	}
	return uid, true, nil
}

func (d *userDirectory) CreateUser(ctx context.Context, name, email, phone string) (int64, error) {
	if !d.IsEnabled() {
		return 0, fmt.Errorf("identity service not enabled")
	}
	return d.svc.CreateUser(ctx, name, email, phone)
}

type operationAccountRegistrar struct {
	svc *OperationAccountService
}

func NewOperationAccountRegistrar(svc *OperationAccountService) iambridge.OperationAccountRegistrar {
	if svc == nil {
		return nil
	}
	return &operationAccountRegistrar{svc: svc}
}

func (r *operationAccountRegistrar) IsEnabled() bool {
	return r != nil && r.svc != nil && r.svc.IsEnabled()
}

func (r *operationAccountRegistrar) RegisterOperationAccount(ctx context.Context, input iambridge.OperationAccountRegistration) (*iambridge.OperationAccountRegistrationResult, error) {
	if !r.IsEnabled() {
		return nil, fmt.Errorf("operation account service not enabled")
	}
	result, err := r.svc.RegisterOperationAccount(ctx, RegisterOperationAccountInput{
		ExistingUserID: formatOptionalInt64(input.ExistingUserID),
		Name:           input.Name,
		Phone:          input.Phone,
		Email:          input.Email,
		ScopedOrgID: formatOptionalInt64(input.ScopedOrgID),
		OperaLoginID:   input.OperaLoginID,
		Password:       input.Password,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return &iambridge.OperationAccountRegistrationResult{
		UserID:       result.UserID,
		AccountID:    result.AccountID,
		CredentialID: result.CredentialID,
		ExternalID:   result.ExternalID,
		IsNewUser:    result.IsNewUser,
		IsNewAccount: result.IsNewAccount,
	}, nil
}

type authzSnapshotReader struct {
	loader *AuthzSnapshotLoader
}

func NewAuthzSnapshotReader(loader *AuthzSnapshotLoader) iambridge.AuthzSnapshotReader {
	if loader == nil {
		return nil
	}
	return &authzSnapshotReader{loader: loader}
}

func (r *authzSnapshotReader) LoadAuthzSnapshot(ctx context.Context, orgID, userID int64) (iambridge.AuthzSnapshot, error) {
	if r == nil || r.loader == nil {
		return nil, fmt.Errorf("iam authorization snapshot loader is not available")
	}
	return r.loader.Load(ctx, r.loader.AuthorizationDomain(), strconv.FormatInt(userID, 10))
}

type operatorAuthzGateway struct {
	assignment *AuthzAssignmentClient
	snapshot   *AuthzSnapshotLoader
}

func NewOperatorAuthzGateway(assignment *AuthzAssignmentClient, snapshot *AuthzSnapshotLoader) iambridge.OperatorAuthzGateway {
	if assignment == nil || snapshot == nil {
		return nil
	}
	return &operatorAuthzGateway{assignment: assignment, snapshot: snapshot}
}

func (g *operatorAuthzGateway) IsEnabled() bool {
	return g != nil && g.assignment != nil && g.snapshot != nil
}

func (g *operatorAuthzGateway) GrantOperatorRole(ctx context.Context, orgID, userID int64, roleName, grantedBy string) error {
	if !g.IsEnabled() {
		return fmt.Errorf("iam operator authorization gateway is not available")
	}
	return g.assignment.Grant(ctx, g.snapshot.DomainForOrg(orgID), strconv.FormatInt(userID, 10), roleName, grantedBy)
}

func (g *operatorAuthzGateway) RevokeOperatorRole(ctx context.Context, orgID, userID int64, roleName string) error {
	if !g.IsEnabled() {
		return fmt.Errorf("iam operator authorization gateway is not available")
	}
	return g.assignment.Revoke(ctx, g.snapshot.DomainForOrg(orgID), strconv.FormatInt(userID, 10), roleName)
}

func (g *operatorAuthzGateway) LoadOperatorRoleNames(ctx context.Context, orgID, userID int64) ([]string, error) {
	if !g.IsEnabled() {
		return nil, fmt.Errorf("iam operator authorization gateway is not available")
	}
	snap, err := g.snapshot.Load(ctx, g.snapshot.DomainForOrg(orgID), strconv.FormatInt(userID, 10))
	if err != nil {
		return nil, err
	}
	if snap == nil {
		return nil, nil
	}
	return snap.RoleNames(), nil
}

type profileLinkDirectory struct {
	profileLink *ProfileLinkService
	identity    *IdentityService
}

func NewProfileLinkDirectory(profileLink *ProfileLinkService, identity *IdentityService) iambridge.ProfileLinkDirectory {
	if profileLink == nil {
		return nil
	}
	return &profileLinkDirectory{profileLink: profileLink, identity: identity}
}

func (d *profileLinkDirectory) IsEnabled() bool {
	return d != nil && d.profileLink != nil && d.profileLink.IsEnabled()
}

func (d *profileLinkDirectory) ListProfileLinkedUsers(ctx context.Context, profileID string) ([]iambridge.ProfileLinkedUser, error) {
	if !d.IsEnabled() {
		return nil, fmt.Errorf("profile link service not enabled")
	}
	resp, err := d.profileLink.ListProfileLinks(ctx, profileID)
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Items) == 0 {
		return []iambridge.ProfileLinkedUser{}, nil
	}

	linkedUsers := make([]iambridge.ProfileLinkedUser, 0, len(resp.Items))
	for _, edge := range resp.Items {
		if edge == nil || edge.ProfileLink == nil {
			continue
		}
		linkedUser := iambridge.ProfileLinkedUser{Relation: edge.ProfileLink.GetRelation().String()}
		user := edge.User
		if user == nil && d.identity != nil && d.identity.IsEnabled() && edge.ProfileLink.UserId != "" {
			userResp, err := d.identity.GetUser(ctx, edge.ProfileLink.UserId)
			if err == nil && userResp != nil {
				user = userResp.User
			}
		}
		if user != nil {
			linkedUser.Name = user.GetNickname()
			linkedUser.Phone = primaryPhone(user)
		}
		linkedUsers = append(linkedUsers, linkedUser)
	}
	return linkedUsers, nil
}

type miniProgramRecipientResolver struct {
	profileLink *ProfileLinkService
	identity    *IdentityService
}

func NewMiniProgramRecipientResolver(profileLink *ProfileLinkService, identity *IdentityService) iambridge.MiniProgramRecipientResolver {
	if profileLink == nil && identity == nil {
		return nil
	}
	return &miniProgramRecipientResolver{profileLink: profileLink, identity: identity}
}

func (r *miniProgramRecipientResolver) IsEnabled() bool {
	return r != nil && ((r.identity != nil && r.identity.IsEnabled()) || (r.profileLink != nil && r.profileLink.IsEnabled()))
}

func (r *miniProgramRecipientResolver) ResolveMiniProgramRecipients(ctx context.Context, profileID string) (*iambridge.MiniProgramRecipients, error) {
	profileID = strings.TrimSpace(profileID)
	if !r.IsEnabled() || profileID == "" {
		return &iambridge.MiniProgramRecipients{}, nil
	}

	if direct := r.resolveUserOpenIDs(ctx, profileID); len(direct) > 0 {
		return &iambridge.MiniProgramRecipients{OpenIDs: direct, Source: "testee"}, nil
	}
	if r.profileLink == nil || !r.profileLink.IsEnabled() {
		return &iambridge.MiniProgramRecipients{}, nil
	}
	resp, err := r.profileLink.ListProfileLinks(ctx, profileID)
	if err != nil {
		return nil, err
	}
	var openIDs []string
	if resp != nil {
		for _, edge := range resp.Items {
			if edge == nil {
				continue
			}
			if edge.User != nil {
				openIDs = append(openIDs, extractMiniProgramOpenIDs(edge.User)...)
				continue
			}
			if edge.ProfileLink != nil && edge.ProfileLink.UserId != "" {
				openIDs = append(openIDs, r.resolveUserOpenIDs(ctx, edge.ProfileLink.UserId)...)
			}
		}
	}
	return &iambridge.MiniProgramRecipients{OpenIDs: uniqueStrings(openIDs), Source: "profile_link"}, nil
}

func (r *miniProgramRecipientResolver) resolveUserOpenIDs(ctx context.Context, userID string) []string {
	if userID == "" || r.identity == nil || !r.identity.IsEnabled() {
		return nil
	}
	resp, err := r.identity.GetUser(ctx, userID)
	if err != nil || resp == nil || resp.User == nil {
		return nil
	}
	return extractMiniProgramOpenIDs(resp.User)
}

func extractMiniProgramOpenIDs(user *identityv2.User) []string {
	if user == nil {
		return nil
	}
	var recipients []string
	for _, identity := range user.ExternalIdentities {
		if identity == nil || identity.ExternalId == "" {
			continue
		}
		provider := strings.ToLower(identity.Provider)
		if provider == "wx:minip" || provider == "wechat" || provider == "wechat_miniprogram" || strings.HasPrefix(provider, "wx:minip:") {
			recipients = append(recipients, identity.ExternalId)
		}
	}
	return uniqueStrings(recipients)
}

func primaryPhone(user *identityv2.User) string {
	if user == nil {
		return ""
	}
	for _, contact := range user.Contacts {
		if contact != nil && contact.GetType().String() == "CONTACT_TYPE_PHONE" {
			return contact.GetValue()
		}
	}
	return ""
}

func uniqueStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	slices.Sort(result)
	return result
}

func formatOptionalInt64(value int64) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}
