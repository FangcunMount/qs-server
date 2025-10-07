package wechat

import (
	"context"
	"fmt"

	accountDomain "github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/account"
	accountPort "github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/account/port"
	wechatDomain "github.com/fangcun-mount/qs-server/internal/apiserver/domain/wechat"
	wechatPort "github.com/fangcun-mount/qs-server/internal/apiserver/domain/wechat/port"
	"github.com/fangcun-mount/qs-server/internal/pkg/code"
	"github.com/fangcun-mount/qs-server/pkg/errors"
)

// WechatAccountCreator 微信账号创建器
// 职责：创建或更新微信账号（小程序、公众号）
type WechatAccountCreator struct {
	wxAccountRepo accountPort.WechatAccountRepository
	appRepo       wechatPort.AppRepository
}

// NewWechatAccountCreator 创建微信账号创建器
func NewWechatAccountCreator(
	wxAccountRepo accountPort.WechatAccountRepository,
	appRepo wechatPort.AppRepository,
) *WechatAccountCreator {
	return &WechatAccountCreator{
		wxAccountRepo: wxAccountRepo,
		appRepo:       appRepo,
	}
}

// CreateOrUpdateMiniProgramAccount 创建或更新小程序账号
// 用于用户第一次使用小程序时创建对应账户
func (c *WechatAccountCreator) CreateOrUpdateMiniProgramAccount(
	ctx context.Context,
	appID string,
	openID string,
	unionID *string,
	nickname string,
	avatar string,
	sessionKey string,
) (*accountDomain.WechatAccount, error) {
	// 1. 验证微信应用
	app, err := c.appRepo.FindByPlatformAndAppID(ctx, wechatDomain.PlatformMini, appID)
	if err != nil {
		return nil, errors.WithCode(code.ErrDatabase, "failed to find wx app: %v", err)
	}
	if !app.IsEnabled() {
		return nil, errors.WithCode(code.ErrValidation, "wx app is disabled")
	}

	// 2. 查找或创建微信账号
	wxAccount, err := c.wxAccountRepo.FindByOpenID(ctx, appID, accountDomain.WxPlatformMini, openID)
	if err != nil {
		// 账号不存在，创建新账号
		wxAccount, err = accountDomain.NewWechatAccount(
			int64(app.ID().Value()),
			appID,
			accountDomain.WxPlatformMini,
			openID,
			unionID,
		)
		if err != nil {
			return nil, errors.WithCode(code.ErrValidation, "failed to create wx account: %v", err)
		}

		// 更新用户信息
		wxAccount.UpdateProfile(nickname, avatar)
		wxAccount.UpdateSessionKey(sessionKey)

		// 保存新账号
		if err := c.wxAccountRepo.Save(ctx, wxAccount); err != nil {
			return nil, fmt.Errorf("failed to save wx account: %w", err)
		}

		return wxAccount, nil
	}

	// 3. 账号已存在，更新信息
	wxAccount.UpdateProfile(nickname, avatar)
	wxAccount.UpdateSessionKey(sessionKey)

	if err := c.wxAccountRepo.Update(ctx, wxAccount); err != nil {
		return nil, fmt.Errorf("failed to update wx account: %w", err)
	}

	return wxAccount, nil
}

// CreateOrUpdateOfficialAccount 创建或更新公众号账号
// 用于用户关注公众号时创建对应的账号
func (c *WechatAccountCreator) CreateOrUpdateOfficialAccount(
	ctx context.Context,
	appID string,
	openID string,
	unionID *string,
	nickname string,
	avatar string,
) (*accountDomain.WechatAccount, error) {
	// 1. 验证微信应用
	app, err := c.appRepo.FindByPlatformAndAppID(ctx, wechatDomain.PlatformOA, appID)
	if err != nil {
		return nil, errors.WithCode(code.ErrDatabase, "failed to find wx app: %v", err)
	}
	if !app.IsEnabled() {
		return nil, errors.WithCode(code.ErrValidation, "wx app is disabled")
	}

	// 2. 查找或创建微信账号
	wxAccount, err := c.wxAccountRepo.FindByOpenID(ctx, appID, accountDomain.WxPlatformOA, openID)
	if err != nil {
		// 账号不存在，创建新账号
		wxAccount, err = accountDomain.NewWechatAccount(
			int64(app.ID().Value()),
			appID,
			accountDomain.WxPlatformOA,
			openID,
			unionID,
		)
		if err != nil {
			return nil, errors.WithCode(code.ErrValidation, "failed to create wx account: %v", err)
		}

		// 更新用户信息
		wxAccount.UpdateProfile(nickname, avatar)

		// 保存新账号
		if err := c.wxAccountRepo.Save(ctx, wxAccount); err != nil {
			return nil, fmt.Errorf("failed to save wx account: %w", err)
		}

		return wxAccount, nil
	}

	// 3. 账号已存在，更新信息
	wxAccount.UpdateProfile(nickname, avatar)

	if err := c.wxAccountRepo.Update(ctx, wxAccount); err != nil {
		return nil, fmt.Errorf("failed to update wx account: %w", err)
	}

	return wxAccount, nil
}

// GetWechatAccountByOpenID 根据 OpenID 获取微信账号
func (c *WechatAccountCreator) GetWechatAccountByOpenID(
	ctx context.Context,
	appID string,
	platform accountDomain.WxPlatform,
	openID string,
) (*accountDomain.WechatAccount, error) {
	return c.wxAccountRepo.FindByOpenID(ctx, appID, platform, openID)
}
