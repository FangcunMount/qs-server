package wechatapi

import (
	"context"
	"fmt"
	"strconv"

	"github.com/silenceper/wechat/v2"
	"github.com/silenceper/wechat/v2/cache"
	miniConfig "github.com/silenceper/wechat/v2/miniprogram/config"
	miniSubscribe "github.com/silenceper/wechat/v2/miniprogram/subscribe"

	wechatmini "github.com/FangcunMount/qs-server/internal/apiserver/port/wechatmini"
)

// SubscribeSender 小程序订阅消息发送器实现。
type SubscribeSender struct {
	cache     cache.Cache
	newClient func(appID, appSecret string) (subscribeClient, error)
}

type subscribeClient interface {
	Send(*miniSubscribe.Message) error
	SendGetMsgID(*miniSubscribe.Message) (int64, error)
	ListTemplates() (*miniSubscribe.TemplateList, error)
}

// NewSubscribeSender 创建小程序订阅消息发送器。
func NewSubscribeSender(sdkCache cache.Cache) *SubscribeSender {
	return &SubscribeSender{cache: sdkCache}
}

// SendSubscribeMessage 发送小程序订阅消息
func (s *SubscribeSender) SendSubscribeMessage(_ context.Context, appID, appSecret string, msg wechatmini.SubscribeMessage) error {
	subscribeClient, err := s.newSubscribeClient(appID, appSecret)
	if err != nil {
		return err
	}
	if err := subscribeClient.Send(subscribeMessage(msg)); err != nil {
		return fmt.Errorf("send subscribe message: %w", err)
	}
	return nil
}

// SendSubscribeMessageWithReceipt returns the platform msgid when the response contains one.
// The upstream library does not accept a context for the send call. In particular, a caller-side
// timeout or lost response must be treated as an unknown outcome, not permission to resend.
func (s *SubscribeSender) SendSubscribeMessageWithReceipt(_ context.Context, appID, appSecret string, msg wechatmini.SubscribeMessage) (wechatmini.SubscribeSendReceipt, error) {
	subscribeClient, err := s.newSubscribeClient(appID, appSecret)
	if err != nil {
		return wechatmini.SubscribeSendReceipt{}, err
	}
	msgID, err := subscribeClient.SendGetMsgID(subscribeMessage(msg))
	if err != nil {
		return wechatmini.SubscribeSendReceipt{}, fmt.Errorf("send subscribe message with receipt: %w", err)
	}
	if msgID <= 0 {
		return wechatmini.SubscribeSendReceipt{}, nil
	}
	return wechatmini.SubscribeSendReceipt{PlatformMessageID: strconv.FormatInt(msgID, 10)}, nil
}

func subscribeMessage(msg wechatmini.SubscribeMessage) *miniSubscribe.Message {
	data := make(map[string]*miniSubscribe.DataItem, len(msg.Data))
	for key, value := range msg.Data {
		data[key] = &miniSubscribe.DataItem{Value: value}
	}
	return &miniSubscribe.Message{
		ToUser:           msg.ToUser,
		TemplateID:       msg.TemplateID,
		Page:             msg.Page,
		Data:             data,
		MiniprogramState: msg.MiniProgramState,
		Lang:             msg.Lang,
	}
}

// ListTemplates 列出小程序订阅消息模板
func (s *SubscribeSender) ListTemplates(_ context.Context, appID, appSecret string) ([]wechatmini.SubscribeTemplate, error) {
	subscribeClient, err := s.newSubscribeClient(appID, appSecret)
	if err != nil {
		return nil, err
	}

	list, err := subscribeClient.ListTemplates()
	if err != nil {
		return nil, fmt.Errorf("list subscribe templates: %w", err)
	}

	templates := make([]wechatmini.SubscribeTemplate, 0, len(list.Data))
	for _, item := range list.Data {
		templates = append(templates, wechatmini.SubscribeTemplate{
			ID:      item.PriTmplID,
			Title:   item.Title,
			Content: item.Content,
		})
	}
	return templates, nil
}

func (s *SubscribeSender) newSubscribeClient(appID, appSecret string) (subscribeClient, error) {
	if appID == "" || appSecret == "" {
		return nil, fmt.Errorf("appID and appSecret cannot be empty")
	}
	if s.newClient != nil {
		return s.newClient(appID, appSecret)
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

var _ wechatmini.MiniProgramSubscribeSender = (*SubscribeSender)(nil)
var _ wechatmini.MiniProgramSubscribeReceiptSender = (*SubscribeSender)(nil)
