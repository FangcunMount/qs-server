package wechat

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/silenceper/wechat/v2/officialaccount/message"

	"github.com/fangcun-mount/qs-server/internal/apiserver/application/wechat"
	wxInfra "github.com/fangcun-mount/qs-server/internal/apiserver/infra/wechat"
	"github.com/fangcun-mount/qs-server/pkg/core"
	"github.com/fangcun-mount/qs-server/pkg/log"
)

// OfficialAccountWebhook 公众号事件回调处理器
type OfficialAccountWebhook struct {
	follower        *wechat.Follower
	wxClientFactory *wxInfra.WxClientFactory
}

// NewOfficialAccountWebhook 创建公众号回调处理器
func NewOfficialAccountWebhook(
	follower *wechat.Follower,
	wxClientFactory *wxInfra.WxClientFactory,
) *OfficialAccountWebhook {
	return &OfficialAccountWebhook{
		follower:        follower,
		wxClientFactory: wxClientFactory,
	}
}

// HandleCallback 处理微信公众号回调
// GET/POST /webhook/wechat/oa/:appId
func (h *OfficialAccountWebhook) HandleCallback(c *gin.Context) {
	appID := c.Param("appId")

	// 1. 获取公众号客户端
	oa, err := h.wxClientFactory.GetOA(c.Request.Context(), appID)
	if err != nil {
		log.Errorw("failed to get oa client", "error", err, "appId", appID)
		core.WriteResponse(c, err, nil)
		return
	}

	// 2. 获取服务器实例
	server := oa.GetServer(c.Request, c.Writer)

	// 3. 设置消息处理器
	server.SetMessageHandler(func(msg *message.MixMessage) *message.Reply {
		return h.handleMessage(c.Request.Context(), appID, msg)
	})

	// 4. 处理请求（验证签名 + 解密消息）
	err = server.Serve()
	if err != nil {
		log.Errorw("failed to serve wechat callback", "error", err, "appId", appID)
		core.WriteResponse(c, err, nil)
		return
	}

	// 5. 发送响应（已由 server.Send() 处理）
	err = server.Send()
	if err != nil {
		log.Errorw("failed to send wechat response", "error", err, "appId", appID)
	}
}

// handleMessage 消息/事件路由
func (h *OfficialAccountWebhook) handleMessage(ctx context.Context, appID string, msg *message.MixMessage) *message.Reply {
	log.Infow("received wechat message",
		"appId", appID,
		"msgType", msg.MsgType,
		"event", msg.Event,
		"fromUser", msg.FromUserName,
	)

	switch msg.MsgType {
	case message.MsgTypeEvent:
		return h.handleEvent(ctx, appID, msg)
	case message.MsgTypeText:
		return h.handleTextMessage(ctx, appID, msg)
	case message.MsgTypeImage:
		return h.handleImageMessage(ctx, appID, msg)
	default:
		log.Infow("unsupported message type", "msgType", msg.MsgType)
		return nil
	}
}

// handleEvent 事件处理
func (h *OfficialAccountWebhook) handleEvent(ctx context.Context, appID string, msg *message.MixMessage) *message.Reply {
	switch msg.Event {
	case message.EventSubscribe:
		return h.HandleSubscribe(ctx, appID, msg)
	case message.EventUnsubscribe:
		return h.HandleUnsubscribe(ctx, appID, msg)
	case message.EventScan:
		return h.HandleScan(ctx, appID, msg)
	case message.EventClick:
		return h.HandleMenuClick(ctx, appID, msg)
	default:
		log.Infow("unsupported event type", "event", msg.Event)
		return nil
	}
}

// HandleSubscribe 处理关注事件
func (h *OfficialAccountWebhook) HandleSubscribe(ctx context.Context, appID string, msg *message.MixMessage) *message.Reply {
	openID := string(msg.FromUserName)
	eventKey := string(msg.EventKey) // 二维码参数（如果是扫码关注）

	log.Infow("user subscribe",
		"appId", appID,
		"openId", openID,
		"eventKey", eventKey,
	)

	// 1. 获取用户信息（通过公众号客户端）
	var nickname, avatar, unionID string
	oa, err := h.wxClientFactory.GetOA(ctx, appID)
	if err != nil {
		log.Errorw("failed to get oa client", "error", err, "appId", appID)
	} else {
		userInfo, err := oa.GetUser().GetUserInfo(openID)
		if err != nil {
			log.Errorw("failed to get user info", "error", err, "openId", openID)
		} else {
			nickname = userInfo.Nickname
			avatar = userInfo.Headimgurl
			unionID = userInfo.UnionID
		}
	}

	// 2. 处理关注逻辑（调用应用服务）
	var unionIDPtr *string
	if unionID != "" {
		unionIDPtr = &unionID
	}

	err = h.follower.HandleSubscribe(ctx, appID, openID, unionIDPtr, nickname, avatar)
	if err != nil {
		log.Errorw("failed to handle subscribe", "error", err, "openId", openID)
	}

	// 3. 返回欢迎消息
	return &message.Reply{
		MsgType: message.MsgTypeText,
		MsgData: message.NewText("感谢关注！欢迎使用我们的服务。"),
	}
}

// HandleUnsubscribe 处理取关事件
func (h *OfficialAccountWebhook) HandleUnsubscribe(ctx context.Context, appID string, msg *message.MixMessage) *message.Reply {
	openID := string(msg.FromUserName)

	log.Infow("user unsubscribe",
		"appId", appID,
		"openId", openID,
	)

	// 处理取关逻辑
	err := h.follower.HandleUnsubscribe(ctx, appID, openID)
	if err != nil {
		log.Errorw("failed to handle unsubscribe", "error", err, "openId", openID)
	}

	// 取关事件无需回复
	return nil
}

// HandleScan 处理扫码事件（已关注用户扫码）
func (h *OfficialAccountWebhook) HandleScan(ctx context.Context, appID string, msg *message.MixMessage) *message.Reply {
	openID := string(msg.FromUserName)
	sceneID := string(msg.EventKey) // 场景值

	log.Infow("user scan",
		"appId", appID,
		"openId", openID,
		"sceneId", sceneID,
	)

	// TODO: 处理扫码逻辑（根据业务需求）
	// 例如：绑定场景、统计等

	return &message.Reply{
		MsgType: message.MsgTypeText,
		MsgData: message.NewText("扫码成功！场景：" + sceneID),
	}
}

// HandleMenuClick 处理菜单点击事件
func (h *OfficialAccountWebhook) HandleMenuClick(ctx context.Context, appID string, msg *message.MixMessage) *message.Reply {
	openID := string(msg.FromUserName)
	menuKey := string(msg.EventKey) // 菜单Key

	log.Infow("user click menu",
		"appId", appID,
		"openId", openID,
		"menuKey", menuKey,
	)

	// TODO: 根据 menuKey 处理不同的菜单点击
	// 可以调用应用服务处理业务逻辑

	return &message.Reply{
		MsgType: message.MsgTypeText,
		MsgData: message.NewText("您点击了菜单：" + menuKey),
	}
}

// handleTextMessage 处理文本消息
func (h *OfficialAccountWebhook) handleTextMessage(ctx context.Context, appID string, msg *message.MixMessage) *message.Reply {
	openID := string(msg.FromUserName)
	content := string(msg.Content)

	log.Infow("received text message",
		"appId", appID,
		"openId", openID,
		"content", content,
	)

	// TODO: 处理文本消息
	// 可以接入智能客服、关键词回复等

	return &message.Reply{
		MsgType: message.MsgTypeText,
		MsgData: message.NewText("收到您的消息：" + content),
	}
}

// handleImageMessage 处理图片消息
func (h *OfficialAccountWebhook) handleImageMessage(ctx context.Context, appID string, msg *message.MixMessage) *message.Reply {
	openID := string(msg.FromUserName)
	picURL := string(msg.PicURL)
	mediaID := string(msg.MediaID)

	log.Infow("received image message",
		"appId", appID,
		"openId", openID,
		"picUrl", picURL,
		"mediaId", mediaID,
	)

	// TODO: 处理图片消息
	// 可以下载图片、识别内容等

	return &message.Reply{
		MsgType: message.MsgTypeText,
		MsgData: message.NewText("收到您的图片"),
	}
}
