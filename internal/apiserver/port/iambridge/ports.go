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
	CreateUser(ctx context.Context, name, email, phone string) (int64, error)
}

type OperationAccountRegistration struct {
	ExistingUserID int64
	Name           string
	Phone          string
	Email          string
	ScopedTenantID int64
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
	GrantOperatorRole(ctx context.Context, orgID, userID int64, roleName, grantedBy string) error
	RevokeOperatorRole(ctx context.Context, orgID, userID int64, roleName string) error
	LoadOperatorRoleNames(ctx context.Context, orgID, userID int64) ([]string, error)
}

type GuardianshipReader interface {
	IsEnabled() bool
	ValidateChildExists(ctx context.Context, childID string) error
}

type Guardian struct {
	Name     string
	Phone    string
	Relation string
}

type GuardianDirectory interface {
	IsEnabled() bool
	ListGuardians(ctx context.Context, childID string) ([]Guardian, error)
}

type MiniProgramRecipients struct {
	OpenIDs []string
	Source  string
}

type MiniProgramRecipientResolver interface {
	IsEnabled() bool
	ResolveMiniProgramRecipients(ctx context.Context, childID string) (*MiniProgramRecipients, error)
}
