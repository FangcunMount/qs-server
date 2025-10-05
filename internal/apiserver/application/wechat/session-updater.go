package wechat

import (
	"context"

	accountDomain "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/account"
	accountPort "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/account/port"
	"github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// SessionUpdater SessionKey更新器
// 职责：更新小程序的SessionKey
type SessionUpdater struct {
	wxAccountRepo accountPort.WechatAccountRepository
}

// NewSessionUpdater 创建SessionKey更新器
func NewSessionUpdater(wxAccountRepo accountPort.WechatAccountRepository) *SessionUpdater {
	return &SessionUpdater{
		wxAccountRepo: wxAccountRepo,
	}
}

// UpdateSessionKey 更新SessionKey
func (s *SessionUpdater) UpdateSessionKey(ctx context.Context, appID, openID, sessionKey string) error {
	wxAcc, err := s.wxAccountRepo.FindByOpenID(ctx, appID, accountDomain.WxPlatformMini, openID)
	if err != nil {
		return errors.WithCode(code.ErrDatabase, "failed to find wx account: %v", err)
	}

	if err := wxAcc.UpdateSessionKey(sessionKey); err != nil {
		return err
	}

	return s.wxAccountRepo.Update(ctx, wxAcc)
}
