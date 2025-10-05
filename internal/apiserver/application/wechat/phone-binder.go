package wechat

import (
	"context"

	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user"
	accountDomain "github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/account"
	accountPort "github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/account/port"
	"github.com/fangcun-mount/qs-server/internal/pkg/code"
	"github.com/fangcun-mount/qs-server/pkg/errors"
	"github.com/fangcun-mount/qs-server/pkg/log"
)

// PhoneBinder 手机号绑定器
// 职责：绑定手机号到用户，并处理账号合并逻辑
type PhoneBinder struct {
	userRepo      user.Repository
	wxAccountRepo accountPort.WechatAccountRepository
	mergeLogRepo  accountPort.MergeLogRepository
}

// NewPhoneBinder 创建手机号绑定器
func NewPhoneBinder(
	userRepo user.Repository,
	wxAccountRepo accountPort.WechatAccountRepository,
	mergeLogRepo accountPort.MergeLogRepository,
) *PhoneBinder {
	return &PhoneBinder{
		userRepo:      userRepo,
		wxAccountRepo: wxAccountRepo,
		mergeLogRepo:  mergeLogRepo,
	}
}

// BindPhone 绑定手机号
func (b *PhoneBinder) BindPhone(ctx context.Context, userID user.UserID, phone string) error {
	// 1. 查找用户
	u, err := b.userRepo.FindByID(ctx, userID)
	if err != nil {
		return errors.WithCode(code.ErrDatabase, "failed to find user: %v", err)
	}

	// 2. 检查手机号是否已被其他用户使用
	existingUser, err := b.userRepo.FindByPhone(ctx, phone)
	if err == nil && existingUser != nil && existingUser.ID() != userID {
		// 手机号已被使用，需要合并决策（这里简单拒绝，实际可以实现合并逻辑）
		return errors.WithCode(code.ErrValidation, "phone already bound to another user")
	}

	// 3. 绑定手机号
	if err := u.ChangePhone(phone); err != nil {
		return err
	}

	// 4. 更新用户
	if err := b.userRepo.Update(ctx, u); err != nil {
		return errors.WithCode(code.ErrDatabase, "failed to update user: %v", err)
	}

	// 5. 查找该用户的所有微信账户，记录合并日志
	wxAccounts, err := b.wxAccountRepo.FindByUserID(ctx, userID)
	if err != nil {
		log.Errorw("failed to find wx accounts", "error", err)
		return nil
	}

	for _, acc := range wxAccounts {
		// 类型断言为 WechatAccount
		wxAcc, ok := acc.(*accountDomain.WechatAccount)
		if !ok {
			continue
		}

		mergeLog := accountDomain.NewMergeLog(userID, wxAcc.GetID(), accountDomain.MergeReasonPhone)
		if err := b.mergeLogRepo.Save(ctx, mergeLog); err != nil {
			log.Errorw("failed to save merge log", "error", err)
		}
	}

	return nil
}
