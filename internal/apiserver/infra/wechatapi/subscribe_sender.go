package wechatapi

import (
	"context"
	"fmt"

	"github.com/silenceper/wechat/v2"
	"github.com/silenceper/wechat/v2/cache"
	miniConfig "github.com/silenceper/wechat/v2/miniprogram/config"
	miniSubscribe "github.com/silenceper/wechat/v2/miniprogram/subscribe"

	wechatPort "github.com/FangcunMount/qs-server/internal/apiserver/infra/wechatapi/port"
)

// SubscribeSender 小程序订阅消息发送器实现。
type SubscribeSender struct {
	cache cache.Cache
}

// NewSubscribeSender 创建小程序订阅消息发送器。
func NewSubscribeSender(sdkCache cache.Cache) *SubscribeSender {
	return &SubscribeSender{cache: sdkCache}
}

func (s *SubscribeSender) SendSubscribeMessage(ctx context.Context, appID, appSecret string, msg wechatPort.SubscribeMessage) error {
	subscribeClient, err := s.newSubscribeClient(appID, appSecret)
	if err != nil {
		return err
	}

	data := make(map[string]*miniSubscribe.DataItem, len(msg.Data))
	for key, value := range msg.Data {
		data[key] = &miniSubscribe.DataItem{Value: value}
	}

	req := &miniSubscribe.Message{
		ToUser:           msg.ToUser,
		TemplateID:       msg.TemplateID,
		Page:             msg.Page,
		Data:             data,
		MiniprogramState: msg.MiniProgramState,
		Lang:             msg.Lang,
	}
	if err := subscribeClient.Send(req); err != nil {
		return fmt.Errorf("send subscribe message: %w", err)
	}
	return nil
}

func (s *SubscribeSender) ListTemplates(ctx context.Context, appID, appSecret string) ([]wechatPort.SubscribeTemplate, error) {
	subscribeClient, err := s.newSubscribeClient(appID, appSecret)
	if err != nil {
		return nil, err
	}

	list, err := subscribeClient.ListTemplates()
	if err != nil {
		return nil, fmt.Errorf("list subscribe templates: %w", err)
	}

	templates := make([]wechatPort.SubscribeTemplate, 0, len(list.Data))
	for _, item := range list.Data {
		templates = append(templates, wechatPort.SubscribeTemplate{
			ID:      item.PriTmplID,
			Title:   item.Title,
			Content: item.Content,
		})
	}
	return templates, nil
}

func (s *SubscribeSender) newSubscribeClient(appID, appSecret string) (*miniSubscribe.Subscribe, error) {
	if appID == "" || appSecret == "" {
		return nil, fmt.Errorf("appID and appSecret cannot be empty")
	}

	wc := wechat.NewWechat()
	cfg := &miniConfig.Config{
		AppID:     appID,
		AppSecret: appSecret,
		Cache:     s.cache,
	}
	miniProgram := wc.GetMiniProgram(cfg)
	return miniProgram.GetSubscribe(), nil
}

var _ wechatPort.MiniProgramSubscribeSender = (*SubscribeSender)(nil)
