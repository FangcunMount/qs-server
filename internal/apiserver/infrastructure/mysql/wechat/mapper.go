package wechat

import (
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/wechat"
)

// AppMapper 微信应用 DO <-> PO 映射器
type AppMapper struct{}

// NewAppMapper 创建映射器
func NewAppMapper() *AppMapper {
	return &AppMapper{}
}

// ToPO 领域对象转持久化对象
func (m *AppMapper) ToPO(app *wechat.App) *AppPO {
	if app == nil {
		return nil
	}

	return &AppPO{
		ID:             app.ID().Value(),
		Name:           app.Name(),
		Platform:       string(app.Platform()),
		AppID:          app.AppID(),
		Secret:         app.Secret(),
		Token:          app.Token(),
		EncodingAESKey: app.EncodingAESKey(),
		MchID:          app.MchID(),
		SerialNo:       app.SerialNo(),
		PayCertID:      app.PayCertID(),
		Env:            string(app.Env()),
		IsEnabled:      app.IsEnabled(),
		Remark:         app.Remark(),
		CreatedAt:      app.CreatedAt(),
		UpdatedAt:      app.UpdatedAt(),
	}
}

// ToDomain 持久化对象转领域对象
func (m *AppMapper) ToDomain(po *AppPO) *wechat.App {
	if po == nil {
		return nil
	}

	return wechat.Reconstitute(
		wechat.NewAppID(po.ID),
		po.Name,
		wechat.Platform(po.Platform),
		po.AppID,
		po.Secret,
		po.Token,
		po.EncodingAESKey,
		po.MchID,
		po.SerialNo,
		po.PayCertID,
		po.IsEnabled,
		wechat.Environment(po.Env),
		po.Remark,
		po.CreatedAt,
		po.UpdatedAt,
	)
}
