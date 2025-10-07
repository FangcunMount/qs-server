package auth

import (
	"context"

	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user"
	accountDomain "github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/account"
	accountPort "github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/account/port"
	wechatDomain "github.com/fangcun-mount/qs-server/internal/apiserver/domain/wechat"
	wechatPort "github.com/fangcun-mount/qs-server/internal/apiserver/domain/wechat/port"
	"github.com/fangcun-mount/qs-server/internal/pkg/code"
	"github.com/fangcun-mount/qs-server/pkg/errors"
	"github.com/fangcun-mount/qs-server/pkg/log"
)

// LoginRequest 微信登录请求
type LoginRequest struct {
	AppID    string  // 微信AppID
	Platform string  // 平台: mini/oa
	Code     string  // 微信登录code
	OpenID   string  // 微信OpenID（code2session后获得）
	UnionID  *string // 微信UnionID（可选）
	Nickname string  // 微信昵称
	Avatar   string  // 微信头像
}

// LoginResponse 微信登录响应
type LoginResponse struct {
	UserID       user.UserID // 用户ID
	IsNewUser    bool        // 是否新用户
	SessionKey   string      // SessionKey（用于解密手机号等）
	NeedBindInfo bool        // 是否需要补充信息
}

// WechatAuthenticator 微信登录认证器
// 职责：处理微信小程序/公众号登录，创建或绑定用户账号
type WechatAuthenticator struct {
	wxAccountRepo accountPort.WechatAccountRepository
	mergeLogRepo  accountPort.MergeLogRepository
	appRepo       wechatPort.AppRepository
	userRepo      user.Repository
}

// NewWechatAuthenticator 创建微信登录认证器
func NewWechatAuthenticator(
	wxAccountRepo accountPort.WechatAccountRepository,
	mergeLogRepo accountPort.MergeLogRepository,
	appRepo wechatPort.AppRepository,
	userRepo user.Repository,
) *WechatAuthenticator {
	return &WechatAuthenticator{
		wxAccountRepo: wxAccountRepo,
		mergeLogRepo:  mergeLogRepo,
		appRepo:       appRepo,
		userRepo:      userRepo,
	}
}

// LoginWithMiniProgram 小程序登录（创建/更新用户的唯一入口）
func (a *WechatAuthenticator) LoginWithMiniProgram(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	// 1. 验证微信应用
	app, err := a.appRepo.FindByPlatformAndAppID(ctx, wechatDomain.PlatformMini, req.AppID)
	if err != nil {
		return nil, errors.WithCode(code.ErrDatabase, "failed to find wx app: %v", err)
	}
	if !app.IsEnabled() {
		return nil, errors.WithCode(code.ErrValidation, "wx app is disabled")
	}

	// 2. Upsert wx_accounts：按 (appid, 'mini', openid) 找记录
	wxAcc, err := a.wxAccountRepo.FindByOpenID(ctx, req.AppID, accountDomain.WxPlatformMini, req.OpenID)
	isNewWxAccount := false

	if err != nil {
		// 没有记录，新建
		wxAcc, err = accountDomain.NewWechatAccount(int64(app.ID().Value()), req.AppID, accountDomain.WxPlatformMini, req.OpenID, req.UnionID)
		if err != nil {
			return nil, errors.WithCode(code.ErrValidation, "failed to create wx account: %v", err)
		}
		wxAcc.UpdateProfile(req.Nickname, req.Avatar)
		wxAcc.RecordLogin()
		isNewWxAccount = true
	} else {
		// 有记录，更新
		if req.UnionID != nil && *req.UnionID != "" {
			wxAcc.UpdateUnionID(*req.UnionID)
		}
		wxAcc.UpdateProfile(req.Nickname, req.Avatar)
		wxAcc.RecordLogin()
	}

	// 3. 绑定到 users（三段式）
	var u *user.User
	var mergeReason accountDomain.MergeReason
	isNewUser := false

	if wxAcc.IsBound() {
		// 已绑定用户，直接查询
		u, err = a.userRepo.FindByID(ctx, *wxAcc.GetUserID())
		if err != nil {
			return nil, errors.WithCode(code.ErrDatabase, "failed to find user: %v", err)
		}
	} else {
		// 未绑定，执行合并逻辑
		u, mergeReason, isNewUser, err = a.findOrCreateUser(ctx, wxAcc, req)
		if err != nil {
			return nil, err
		}

		// 绑定用户
		wxAcc.BindUser(u.ID())
	}

	// 4. 保存/更新
	if isNewWxAccount {
		if err := a.wxAccountRepo.Save(ctx, wxAcc); err != nil {
			return nil, errors.WithCode(code.ErrDatabase, "failed to save wx account: %v", err)
		}
	} else {
		if err := a.wxAccountRepo.Update(ctx, wxAcc); err != nil {
			return nil, errors.WithCode(code.ErrDatabase, "failed to update wx account: %v", err)
		}
	}

	if isNewUser {
		if err := a.userRepo.Save(ctx, u); err != nil {
			return nil, errors.WithCode(code.ErrDatabase, "failed to save user: %v", err)
		}
	}

	// 5. 记录合并日志
	if !wxAcc.IsBound() && mergeReason != "" {
		mergeLog := accountDomain.NewMergeLog(u.ID(), wxAcc.GetID(), mergeReason)
		if err := a.mergeLogRepo.Save(ctx, mergeLog); err != nil {
			log.Errorw("failed to save merge log", "error", err)
		}
	}

	return &LoginResponse{
		UserID:       u.ID(),
		IsNewUser:    isNewUser,
		SessionKey:   wxAcc.SessionKey(),
		NeedBindInfo: u.Phone() == "",
	}, nil
}

// findOrCreateUser 查找或创建用户（三段式合并逻辑）
func (a *WechatAuthenticator) findOrCreateUser(
	ctx context.Context,
	wxAcc *accountDomain.WechatAccount,
	req *LoginRequest,
) (*user.User, accountDomain.MergeReason, bool, error) {
	// 1. 若 unionid 存在 → 查找同 unionid 的其他账号
	if wxAcc.UnionID() != nil && *wxAcc.UnionID() != "" {
		boundAcc, err := a.wxAccountRepo.FindBoundAccountByUnionID(ctx, *wxAcc.UnionID())
		if err == nil && boundAcc != nil {
			// 找到已绑定的账号，复用同一 user_id
			u, err := a.userRepo.FindByID(ctx, *boundAcc.GetUserID())
			if err == nil {
				return u, accountDomain.MergeReasonUnionID, false, nil
			}
		}
	}

	// 2. 创建新用户（使用微信信息）
	u := user.NewUserWithWechatInfo(req.Nickname, req.Avatar)
	return u, "", true, nil
}
