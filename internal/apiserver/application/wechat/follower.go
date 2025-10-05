package wechat

import (
	"context"

	accountDomain "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/account"
	accountPort "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/account/port"
	wechatDomain "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/wechat"
	wechatPort "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/wechat/port"
	"github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// Follower 公众号关注/取关处理器
// 职责：处理公众号的关注和取关事件
type Follower struct {
	wxAccountRepo accountPort.WechatAccountRepository
	mergeLogRepo  accountPort.MergeLogRepository
	appRepo       wechatPort.AppRepository
}

// NewFollower 创建公众号关注/取关处理器
func NewFollower(
	wxAccountRepo accountPort.WechatAccountRepository,
	mergeLogRepo accountPort.MergeLogRepository,
	appRepo wechatPort.AppRepository,
) *Follower {
	return &Follower{
		wxAccountRepo: wxAccountRepo,
		mergeLogRepo:  mergeLogRepo,
		appRepo:       appRepo,
	}
}

// HandleSubscribe 处理公众号关注事件
func (f *Follower) HandleSubscribe(
	ctx context.Context,
	appID string,
	openID string,
	unionID *string,
	nickname, avatar string,
) error {
	// 1. 验证微信应用
	app, err := f.appRepo.FindByPlatformAndAppID(ctx, wechatDomain.PlatformOA, appID)
	if err != nil {
		return errors.WithCode(code.ErrDatabase, "failed to find wx app: %v", err)
	}
	if !app.IsEnabled() {
		return errors.WithCode(code.ErrValidation, "wx app is disabled")
	}

	// 2. Upsert wx_accounts
	wxAcc, err := f.wxAccountRepo.FindByOpenID(ctx, appID, accountDomain.WxPlatformOA, openID)
	isNew := false

	if err != nil {
		// 新建账号
		wxAcc, err = accountDomain.NewWechatAccount(int64(app.ID().Value()), appID, accountDomain.WxPlatformOA, openID, unionID)
		if err != nil {
			return errors.WithCode(code.ErrValidation, "failed to create wx account: %v", err)
		}
		wxAcc.UpdateProfile(nickname, avatar)
		isNew = true
	} else {
		// 更新
		if unionID != nil && *unionID != "" {
			wxAcc.UpdateUnionID(*unionID)
		}
		wxAcc.UpdateProfile(nickname, avatar)
	}

	// 3. 标记关注
	if err := wxAcc.Follow(); err != nil {
		return err
	}

	// 4. 若有 unionid，尝试绑定到已有用户
	if !wxAcc.IsBound() && wxAcc.UnionID() != nil && *wxAcc.UnionID() != "" {
		boundAcc, err := f.wxAccountRepo.FindBoundAccountByUnionID(ctx, *wxAcc.UnionID())
		if err == nil && boundAcc != nil {
			wxAcc.BindUser(*boundAcc.GetUserID())

			// 记录合并日志
			mergeLog := accountDomain.NewMergeLog(*boundAcc.GetUserID(), wxAcc.GetID(), accountDomain.MergeReasonUnionID)
			if err := f.mergeLogRepo.Save(ctx, mergeLog); err != nil {
				log.Errorw("failed to save merge log", "error", err)
			}
		}
	}

	// 5. 保存/更新
	if isNew {
		return f.wxAccountRepo.Save(ctx, wxAcc)
	}
	return f.wxAccountRepo.Update(ctx, wxAcc)
}

// HandleUnsubscribe 处理公众号取关事件
func (f *Follower) HandleUnsubscribe(ctx context.Context, appID string, openID string) error {
	// 1. 查找账号
	wxAcc, err := f.wxAccountRepo.FindByOpenID(ctx, appID, accountDomain.WxPlatformOA, openID)
	if err != nil {
		return errors.WithCode(code.ErrDatabase, "failed to find wx account: %v", err)
	}

	// 2. 标记取关（不删账号、不解绑用户）
	if err := wxAcc.Unfollow(); err != nil {
		return err
	}

	// 3. 更新
	return f.wxAccountRepo.Update(ctx, wxAcc)
}
