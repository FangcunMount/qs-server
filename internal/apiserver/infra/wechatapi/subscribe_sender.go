package wechatapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/silenceper/wechat/v2"
	"github.com/silenceper/wechat/v2/cache"
	miniConfig "github.com/silenceper/wechat/v2/miniprogram/config"
	miniSubscribe "github.com/silenceper/wechat/v2/miniprogram/subscribe"
	"github.com/silenceper/wechat/v2/util"

	wechatmini "github.com/FangcunMount/qs-server/internal/apiserver/port/wechatmini"
)

// SubscribeSender 小程序订阅消息发送器实现。
type SubscribeSender struct {
	cache     cache.Cache
	newClient func(appID, appSecret string) (subscribeClient, error)
	doRequest func(*http.Request) (*http.Response, error)
}

type subscribeClient interface {
	Send(*miniSubscribe.Message) error
	GetAccessTokenContext(context.Context) (string, error)
	ListTemplates() (*miniSubscribe.TemplateList, error)
}

const subscribeSendURL = "https://api.weixin.qq.com/cgi-bin/message/subscribe/send"

type subscribeSendResponse struct {
	ErrCode *int64  `json:"errcode"`
	ErrMsg  *string `json:"errmsg"`
	MsgID   *int64  `json:"msgid"`
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

// SendSubscribeMessageWithReceipt records an accepted platform API response and
// its msgid when present. The documented subscribe-message success response is
// {"errcode":0,"errmsg":"ok"} without a msgid.
// https://developers.weixin.qq.com/miniprogram/dev/server/API/mp-message-management/subscribe-message/api_sendmessage
// The upstream SendGetMsgID method treats a missing errcode as zero. Read the
// response directly so only an explicit platform success can confirm the send.
// A timeout or lost response remains unknown, never permission to resend.
func (s *SubscribeSender) SendSubscribeMessageWithReceipt(ctx context.Context, appID, appSecret string, msg wechatmini.SubscribeMessage) (wechatmini.SubscribeSendReceipt, error) {
	subscribeClient, err := s.newSubscribeClient(appID, appSecret)
	if err != nil {
		return wechatmini.SubscribeSendReceipt{}, err
	}
	accessToken, err := subscribeClient.GetAccessTokenContext(ctx)
	if err != nil {
		return wechatmini.SubscribeSendReceipt{}, fmt.Errorf("get subscribe message access token: %w", err)
	}
	endpoint := subscribeSendURL + "?access_token=" + url.QueryEscape(accessToken)
	body, err := json.Marshal(subscribeMessage(msg))
	if err != nil {
		return wechatmini.SubscribeSendReceipt{}, fmt.Errorf("encode subscribe message: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return wechatmini.SubscribeSendReceipt{}, fmt.Errorf("create subscribe request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json;charset=utf-8")
	doRequest := s.doRequest
	if doRequest == nil {
		doRequest = util.DefaultHTTPClient.Do
	}
	response, err := doRequest(req)
	if err != nil {
		// HTTP errors may contain the request URL and its access token.
		return wechatmini.SubscribeSendReceipt{}, fmt.Errorf("send subscribe request: response unavailable")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return wechatmini.SubscribeSendReceipt{}, fmt.Errorf("send subscribe request: HTTP %d", response.StatusCode)
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 64*1024+1))
	if err != nil || len(responseBody) > 64*1024 {
		return wechatmini.SubscribeSendReceipt{}, fmt.Errorf("read subscribe response: incomplete or too large")
	}
	var result subscribeSendResponse
	if err := json.Unmarshal(responseBody, &result); err != nil || result.ErrCode == nil {
		return wechatmini.SubscribeSendReceipt{}, fmt.Errorf("send subscribe request: missing or invalid errcode")
	}
	if *result.ErrCode != 0 {
		return wechatmini.SubscribeSendReceipt{}, fmt.Errorf("send subscribe request: platform errcode=%d", *result.ErrCode)
	}
	if result.ErrMsg == nil || *result.ErrMsg != "ok" {
		return wechatmini.SubscribeSendReceipt{}, fmt.Errorf("send subscribe request: missing or invalid success message")
	}
	receipt := wechatmini.SubscribeSendReceipt{Accepted: true}
	if result.MsgID != nil {
		if *result.MsgID <= 0 {
			return wechatmini.SubscribeSendReceipt{}, fmt.Errorf("send subscribe request: invalid msgid")
		}
		receipt.PlatformMessageID = strconv.FormatInt(*result.MsgID, 10)
	}
	return receipt, nil
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
