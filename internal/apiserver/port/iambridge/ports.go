package iambridge

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type WeChatAppConfig struct {
	AppID     string
	AppSecret string
}

type WeChatAppConfigProvider interface {
	IsEnabled() bool
	ResolveWeChatAppConfig(ctx context.Context, wechatAppID string) (*WeChatAppConfig, error)
}

type IdentityResolver interface {
	IsEnabled() bool
	ResolveUserNames(ctx context.Context, ids []meta.ID) map[string]string
}

type UserDirectory interface {
	IsEnabled() bool
	FindUserIDByPhone(ctx context.Context, phone string) (int64, bool, error)
}

type OperationAccountRegistration struct {
	ExistingUserID int64
	Name           string
	Phone          string
	Email          string
	ScopedOrgID    int64
	OperaLoginID   string
	Password       string
}

type OperationAccountRegistrationResult struct {
	UserID       int64
	AccountID    string
	CredentialID string
	ExternalID   string
	IsNewUser    bool
	IsNewAccount bool
}

type OperationAccountRegistrar interface {
	IsEnabled() bool
	RegisterOperationAccount(ctx context.Context, input OperationAccountRegistration) (*OperationAccountRegistrationResult, error)
}

type AuthzSnapshot interface {
	IsQSAdmin() bool
}

type AuthzSnapshotReader interface {
	LoadAuthzSnapshot(ctx context.Context, orgID, userID int64) (AuthzSnapshot, error)
}

type OperatorAuthzGateway interface {
	IsEnabled() bool
	ReplaceManagedOperatorRoles(ctx context.Context, orgID, userID int64, roleNames []string, changedBy, reason string) (int64, error)
	LoadOperatorRoleProjection(ctx context.Context, orgID, userID int64) (OperatorRoleProjection, error)
}

type OperatorRoleProjection struct {
	DirectRoles    []string
	EffectiveRoles []string
	PolicyVersion  int64
}

type ProfileReader interface {
	IsEnabled() bool
	ValidateProfileExists(ctx context.Context, profileID string) error
}

type ProfileLinkedUser struct {
	Name     string
	Phone    string
	Relation string
}

type ProfileLinkDirectory interface {
	IsEnabled() bool
	ListProfileLinkedUsers(ctx context.Context, profileID string) ([]ProfileLinkedUser, error)
}

type MiniProgramRecipients struct {
	OpenIDs []string
	Source  string
}

type MiniProgramRecipientResolver interface {
	IsEnabled() bool
	ResolveMiniProgramRecipients(ctx context.Context, profileID string) (*MiniProgramRecipients, error)
}
