package account

import (
	"time"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// WxPlatform 微信平台类型
type WxPlatform string

const (
	WxPlatformMini WxPlatform = "mini" // 小程序
	WxPlatformOA   WxPlatform = "oa"   // 公众号
)

// WechatAccount 微信账户（聚合根）
type WechatAccount struct {
	*BaseAccount // 继承基础账户

	// 微信特有属性
	appID      int64      // 关联的微信应用ID
	wxAppID    string     // 微信AppID
	platform   WxPlatform // 平台类型
	openID     string     // OpenID
	unionID    *string    // UnionID（可选，用于跨平台合并）
	nickname   string     // 微信昵称
	avatarURL  string     // 微信头像
	sessionKey string     // SessionKey（小程序用）

	// 公众号特有
	followed     bool       // 是否关注（仅OA）
	followedAt   *time.Time // 关注时间
	unfollowedAt *time.Time // 取关时间

	// 活跃度
	lastLoginAt *time.Time // 最近登录时间
}

// NewWechatAccount 创建微信账户
func NewWechatAccount(
	appID int64,
	wxAppID string,
	platform WxPlatform,
	openID string,
	unionID *string,
) (*WechatAccount, error) {
	if openID == "" {
		return nil, errors.WithCode(code.ErrValidation, "openID cannot be empty")
	}
	if wxAppID == "" {
		return nil, errors.WithCode(code.ErrValidation, "wxAppID cannot be empty")
	}

	return &WechatAccount{
		BaseAccount: NewBaseAccount(TypeWechat),
		appID:       appID,
		wxAppID:     wxAppID,
		platform:    platform,
		openID:      openID,
		unionID:     unionID,
		followed:    false,
	}, nil
}

// Reconstitute 从持久化数据重建微信账户
func ReconstituteWechatAccount(
	id AccountID,
	userID *user.UserID,
	appID int64,
	wxAppID string,
	platform WxPlatform,
	openID string,
	unionID *string,
	nickname, avatarURL, sessionKey string,
	followed bool,
	followedAt, unfollowedAt, lastLoginAt *time.Time,
	isActive bool,
	createdAt, updatedAt time.Time,
) *WechatAccount {
	base := &BaseAccount{
		id:        id,
		userID:    userID,
		accType:   TypeWechat,
		isActive:  isActive,
		createdAt: createdAt,
		updatedAt: updatedAt,
	}

	return &WechatAccount{
		BaseAccount:  base,
		appID:        appID,
		wxAppID:      wxAppID,
		platform:     platform,
		openID:       openID,
		unionID:      unionID,
		nickname:     nickname,
		avatarURL:    avatarURL,
		sessionKey:   sessionKey,
		followed:     followed,
		followedAt:   followedAt,
		unfollowedAt: unfollowedAt,
		lastLoginAt:  lastLoginAt,
	}
}

// Getters
func (w *WechatAccount) AppID() int64             { return w.appID }
func (w *WechatAccount) WxAppID() string          { return w.wxAppID }
func (w *WechatAccount) Platform() WxPlatform     { return w.platform }
func (w *WechatAccount) OpenID() string           { return w.openID }
func (w *WechatAccount) UnionID() *string         { return w.unionID }
func (w *WechatAccount) Nickname() string         { return w.nickname }
func (w *WechatAccount) AvatarURL() string        { return w.avatarURL }
func (w *WechatAccount) SessionKey() string       { return w.sessionKey }
func (w *WechatAccount) Followed() bool           { return w.followed }
func (w *WechatAccount) FollowedAt() *time.Time   { return w.followedAt }
func (w *WechatAccount) UnfollowedAt() *time.Time { return w.unfollowedAt }
func (w *WechatAccount) LastLoginAt() *time.Time  { return w.lastLoginAt }

// UpdateUnionID 更新UnionID
func (w *WechatAccount) UpdateUnionID(unionID string) {
	if unionID != "" {
		w.unionID = &unionID
	}
}

// UpdateProfile 更新资料
func (w *WechatAccount) UpdateProfile(nickname, avatarURL string) {
	if nickname != "" {
		w.nickname = nickname
	}
	if avatarURL != "" {
		w.avatarURL = avatarURL
	}
}

// UpdateSessionKey 更新SessionKey（小程序）
func (w *WechatAccount) UpdateSessionKey(sessionKey string) error {
	if w.platform != WxPlatformMini {
		return errors.WithCode(code.ErrValidation, "session key only for mini program")
	}
	w.sessionKey = sessionKey
	return nil
}

// RecordLogin 记录登录
func (w *WechatAccount) RecordLogin() {
	now := time.Now()
	w.lastLoginAt = &now
}

// Follow 关注（公众号）
func (w *WechatAccount) Follow() error {
	if w.platform != WxPlatformOA {
		return errors.WithCode(code.ErrValidation, "follow only for OA platform")
	}
	w.followed = true
	now := time.Now()
	w.followedAt = &now
	w.unfollowedAt = nil
	return nil
}

// Unfollow 取关（公众号）
func (w *WechatAccount) Unfollow() error {
	if w.platform != WxPlatformOA {
		return errors.WithCode(code.ErrValidation, "unfollow only for OA platform")
	}
	w.followed = false
	now := time.Now()
	w.unfollowedAt = &now
	return nil
}

// IsMiniProgram 是否为小程序账户
func (w *WechatAccount) IsMiniProgram() bool {
	return w.platform == WxPlatformMini
}

// IsOfficialAccount 是否为公众号账户
func (w *WechatAccount) IsOfficialAccount() bool {
	return w.platform == WxPlatformOA
}
