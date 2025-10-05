package account

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/account"
)

// WechatAccountMapper 微信账户 DO <-> PO 映射器
type WechatAccountMapper struct{}

// NewWechatAccountMapper 创建映射器
func NewWechatAccountMapper() *WechatAccountMapper {
	return &WechatAccountMapper{}
}

// ToPO 领域对象转持久化对象
func (m *WechatAccountMapper) ToPO(wxAcc *account.WechatAccount) *WechatAccountPO {
	if wxAcc == nil {
		return nil
	}

	var userID *uint64
	if wxAcc.GetUserID() != nil {
		id := wxAcc.GetUserID().Value()
		userID = &id
	}

	return &WechatAccountPO{
		ID:           wxAcc.GetID().Value(),
		UserID:       userID,
		AppID:        wxAcc.AppID(),
		WxAppID:      wxAcc.WxAppID(),
		Platform:     string(wxAcc.Platform()),
		OpenID:       wxAcc.OpenID(),
		UnionID:      wxAcc.UnionID(),
		Nickname:     wxAcc.Nickname(),
		AvatarURL:    wxAcc.AvatarURL(),
		SessionKey:   wxAcc.SessionKey(),
		Followed:     wxAcc.Followed(),
		FollowedAt:   wxAcc.FollowedAt(),
		UnfollowedAt: wxAcc.UnfollowedAt(),
		LastLoginAt:  wxAcc.LastLoginAt(),
		IsActive:     wxAcc.IsActive(),
		CreatedAt:    wxAcc.CreatedAt(),
		UpdatedAt:    wxAcc.UpdatedAt(),
	}
}

// ToDomain 持久化对象转领域对象
func (m *WechatAccountMapper) ToDomain(po *WechatAccountPO) *account.WechatAccount {
	if po == nil {
		return nil
	}

	var userID *user.UserID
	if po.UserID != nil {
		id := user.NewUserID(*po.UserID)
		userID = &id
	}

	return account.ReconstituteWechatAccount(
		account.NewAccountID(po.ID),
		userID,
		po.AppID,
		po.WxAppID,
		account.WxPlatform(po.Platform),
		po.OpenID,
		po.UnionID,
		po.Nickname,
		po.AvatarURL,
		po.SessionKey,
		po.Followed,
		po.FollowedAt,
		po.UnfollowedAt,
		po.LastLoginAt,
		po.IsActive,
		po.CreatedAt,
		po.UpdatedAt,
	)
}

// MergeLogMapper 合并日志 DO <-> PO 映射器
type MergeLogMapper struct{}

// NewMergeLogMapper 创建映射器
func NewMergeLogMapper() *MergeLogMapper {
	return &MergeLogMapper{}
}

// ToPO 领域对象转持久化对象
func (m *MergeLogMapper) ToPO(log *account.MergeLog) *MergeLogPO {
	if log == nil {
		return nil
	}

	return &MergeLogPO{
		ID:        log.ID().Value(),
		UserID:    log.UserID().Value(),
		AccountID: log.AccountID().Value(),
		Reason:    string(log.Reason()),
		CreatedAt: log.CreatedAt(),
	}
}

// ToDomain 持久化对象转领域对象
func (m *MergeLogMapper) ToDomain(po *MergeLogPO) *account.MergeLog {
	if po == nil {
		return nil
	}

	return account.ReconstituteMergeLog(
		account.NewMergeLogID(po.ID),
		user.NewUserID(po.UserID),
		account.NewAccountID(po.AccountID),
		account.MergeReason(po.Reason),
		po.CreatedAt,
	)
}
