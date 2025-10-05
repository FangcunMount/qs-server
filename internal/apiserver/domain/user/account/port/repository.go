package port

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/account"
)

// AccountRepository 账户仓储接口（出站端口）
type AccountRepository interface {
	// Save 保存账户
	Save(ctx context.Context, acc account.Account) error

	// Update 更新账户
	Update(ctx context.Context, acc account.Account) error

	// FindByID 根据ID查找账户
	FindByID(ctx context.Context, id account.AccountID) (account.Account, error)

	// FindByUserID 根据用户ID查找所有账户
	FindByUserID(ctx context.Context, userID user.UserID) ([]account.Account, error)

	// Delete 删除账户
	Delete(ctx context.Context, id account.AccountID) error
}

// WechatAccountRepository 微信账户仓储接口（出站端口）
type WechatAccountRepository interface {
	AccountRepository

	// FindByOpenID 根据OpenID查找微信账户
	FindByOpenID(ctx context.Context, wxAppID string, platform account.WxPlatform, openID string) (*account.WechatAccount, error)

	// FindByUnionID 根据UnionID查找所有微信账户
	FindByUnionID(ctx context.Context, unionID string) ([]*account.WechatAccount, error)

	// FindBoundAccountByUnionID 根据UnionID查找已绑定用户的微信账户
	FindBoundAccountByUnionID(ctx context.Context, unionID string) (*account.WechatAccount, error)

	// ExistsByOpenID 检查OpenID是否存在
	ExistsByOpenID(ctx context.Context, wxAppID string, platform account.WxPlatform, openID string) (bool, error)
}

// MergeLogRepository 账号合并日志仓储接口（出站端口）
type MergeLogRepository interface {
	// Save 保存合并日志
	Save(ctx context.Context, log *account.MergeLog) error

	// FindByUserID 根据用户ID查找合并日志
	FindByUserID(ctx context.Context, userID user.UserID) ([]*account.MergeLog, error)

	// FindByAccountID 根据账户ID查找合并日志
	FindByAccountID(ctx context.Context, accountID account.AccountID) ([]*account.MergeLog, error)
}
