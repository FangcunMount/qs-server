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
		ScopedTenantID: formatOptionalInt64(input.ScopedTenantID),
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
	return r.loader.Load(ctx, strconv.FormatInt(orgID, 10), strconv.FormatInt(userID, 10))
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

type guardianDirectory struct {
	guardianship *GuardianshipService
	identity     *IdentityService
}

func NewGuardianDirectory(guardianship *GuardianshipService, identity *IdentityService) iambridge.GuardianDirectory {
	if guardianship == nil {
		return nil
	}
	return &guardianDirectory{guardianship: guardianship, identity: identity}
}

func (d *guardianDirectory) IsEnabled() bool {
	return d != nil && d.guardianship != nil && d.guardianship.IsEnabled()
}

func (d *guardianDirectory) ListGuardians(ctx context.Context, childID string) ([]iambridge.Guardian, error) {
	if !d.IsEnabled() {
		return nil, fmt.Errorf("guardianship service not enabled")
	}
	resp, err := d.guardianship.ListGuardians(ctx, childID)
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Items) == 0 {
		return []iambridge.Guardian{}, nil
	}

	guardians := make([]iambridge.Guardian, 0, len(resp.Items))
	for _, edge := range resp.Items {
		if edge == nil || edge.ProfileLink == nil {
			continue
		}
		guardian := iambridge.Guardian{Relation: edge.ProfileLink.GetRelation().String()}
		user := edge.User
		if user == nil && d.identity != nil && d.identity.IsEnabled() && edge.ProfileLink.UserId != "" {
			userResp, err := d.identity.GetUser(ctx, edge.ProfileLink.UserId)
			if err == nil && userResp != nil {
				user = userResp.User
			}
		}
		if user != nil {
			guardian.Name = user.GetNickname()
			guardian.Phone = primaryPhone(user)
		}
		guardians = append(guardians, guardian)
	}
	return guardians, nil
}

type miniProgramRecipientResolver struct {
	guardianship *GuardianshipService
	identity     *IdentityService
}

func NewMiniProgramRecipientResolver(guardianship *GuardianshipService, identity *IdentityService) iambridge.MiniProgramRecipientResolver {
	if guardianship == nil && identity == nil {
		return nil
	}
	return &miniProgramRecipientResolver{guardianship: guardianship, identity: identity}
}

func (r *miniProgramRecipientResolver) IsEnabled() bool {
	return r != nil && ((r.identity != nil && r.identity.IsEnabled()) || (r.guardianship != nil && r.guardianship.IsEnabled()))
}

func (r *miniProgramRecipientResolver) ResolveMiniProgramRecipients(ctx context.Context, childID string) (*iambridge.MiniProgramRecipients, error) {
	childID = strings.TrimSpace(childID)
	if !r.IsEnabled() || childID == "" {
		return &iambridge.MiniProgramRecipients{}, nil
	}

	if direct := r.resolveUserOpenIDs(ctx, childID); len(direct) > 0 {
		return &iambridge.MiniProgramRecipients{OpenIDs: direct, Source: "testee"}, nil
	}
	if r.guardianship == nil || !r.guardianship.IsEnabled() {
		return &iambridge.MiniProgramRecipients{}, nil
	}
	resp, err := r.guardianship.ListGuardians(ctx, childID)
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
	return &iambridge.MiniProgramRecipients{OpenIDs: uniqueStrings(openIDs), Source: "guardian"}, nil
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
