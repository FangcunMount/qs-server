package wechat

import (
	"context"

	accountDomain "github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/account"
	accountPort "github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/account/port"
	wechatDomain "github.com/fangcun-mount/qs-server/internal/apiserver/domain/wechat"
	wechatPort "github.com/fangcun-mount/qs-server/internal/apiserver/domain/wechat/port"
	"github.com/fangcun-mount/qs-server/internal/pkg/code"
	"github.com/fangcun-mount/qs-server/pkg/errors"
	"github.com/fangcun-mount/qs-server/pkg/log"
)

// Subscriber 公众号订阅管理器
// 职责：处理公众号的关注和取关事件业务逻辑
type Subscriber struct {
	wxAccountRepo accountPort.WechatAccountRepository
	mergeLogRepo  accountPort.MergeLogRepository
	appRepo       wechatPort.AppRepository
}

// NewSubscriber 创建订阅管理器
func NewSubscriber(
	wxAccountRepo accountPort.WechatAccountRepository,
	mergeLogRepo accountPort.MergeLogRepository,
	appRepo wechatPort.AppRepository,
) *Subscriber {
	return &Subscriber{
		wxAccountRepo: wxAccountRepo,
		mergeLogRepo:  mergeLogRepo,
		appRepo:       appRepo,
	}
}

// Subscribe 处理关注事件
// 返回 error 表示处理失败（但不影响用户体验）
func (s *Subscriber) Subscribe(
	ctx context.Context,
	appID string,
	openID string,
	unionID *string,
	nickname, avatar string,
) error {
	// 1. 验证微信应用
	app, err := s.appRepo.FindByPlatformAndAppID(ctx, wechatDomain.PlatformOA, appID)
	if err != nil {
		return errors.WithCode(code.ErrDatabase, "failed to find wx app: %v", err)
	}
	if !app.IsEnabled() {
		return errors.WithCode(code.ErrValidation, "wx app is disabled")
	}

	// 2. 查找或创建微信账号
	wxAcc, err := s.wxAccountRepo.FindByOpenID(ctx, appID, accountDomain.WxPlatformOA, openID)
	isNew := false

	if err != nil {
		// 新建账号
		wxAcc, err = accountDomain.NewWechatAccount(
			int64(app.ID().Value()),
			appID,
			accountDomain.WxPlatformOA,
			openID,
			unionID,
		)
		if err != nil {
			return errors.WithCode(code.ErrValidation, "failed to create wx account: %v", err)
		}
		wxAcc.UpdateProfile(nickname, avatar)
		isNew = true
	} else {
		// 更新现有账号
		if unionID != nil && *unionID != "" {
			wxAcc.UpdateUnionID(*unionID)
		}
		wxAcc.UpdateProfile(nickname, avatar)
	}

	// 3. 标记为已关注
	if err := wxAcc.Follow(); err != nil {
		return errors.WithCode(code.ErrValidation, "failed to follow: %v", err)
	}

	// 4. 若有 UnionID 且未绑定用户，尝试绑定到已有用户
	if !wxAcc.IsBound() && wxAcc.UnionID() != nil && *wxAcc.UnionID() != "" {
		boundAcc, err := s.wxAccountRepo.FindBoundAccountByUnionID(ctx, *wxAcc.UnionID())
		if err == nil && boundAcc != nil {
			// 找到已绑定的账号，绑定到同一用户
			wxAcc.BindUser(*boundAcc.GetUserID())

			// 记录合并日志
			mergeLog := accountDomain.NewMergeLog(
				*boundAcc.GetUserID(),
				wxAcc.GetID(),
				accountDomain.MergeReasonUnionID,
			)
			if err := s.mergeLogRepo.Save(ctx, mergeLog); err != nil {
				log.Errorw("failed to save merge log", "error", err)
			}
		}
	}

	// 5. 保存或更新账号
	if isNew {
		return s.wxAccountRepo.Save(ctx, wxAcc)
	}
	return s.wxAccountRepo.Update(ctx, wxAcc)
}

// Unsubscribe 处理取关事件
// 注意：不删除账号，不解绑用户，只标记为未关注状态
func (s *Subscriber) Unsubscribe(
	ctx context.Context,
	appID string,
	openID string,
) error {
	// 1. 查找账号
	wxAcc, err := s.wxAccountRepo.FindByOpenID(ctx, appID, accountDomain.WxPlatformOA, openID)
	if err != nil {
		return errors.WithCode(code.ErrDatabase, "failed to find wx account: %v", err)
	}

	// 2. 标记为未关注
	if err := wxAcc.Unfollow(); err != nil {
		return errors.WithCode(code.ErrValidation, "failed to unfollow: %v", err)
	}

	// 3. 更新账号
	return s.wxAccountRepo.Update(ctx, wxAcc)
}
