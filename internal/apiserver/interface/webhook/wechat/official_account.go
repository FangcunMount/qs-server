package wechat

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/silenceper/wechat/v2/officialaccount/message"

	accountDomain "github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/account"
	accountPort "github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/account/port"
	wechatDomain "github.com/fangcun-mount/qs-server/internal/apiserver/domain/wechat"
	wechatPort "github.com/fangcun-mount/qs-server/internal/apiserver/domain/wechat/port"
	wxInfra "github.com/fangcun-mount/qs-server/internal/apiserver/infra/wechat"
	"github.com/fangcun-mount/qs-server/pkg/core"
	"github.com/fangcun-mount/qs-server/pkg/log"
)

// OfficialAccountWebhook 公众号事件回调处理器
type OfficialAccountWebhook struct {
	wxClientFactory *wxInfra.WxClientFactory
	wxAccountRepo   accountPort.WechatAccountRepository
	mergeLogRepo    accountPort.MergeLogRepository
	appRepo         wechatPort.AppRepository
}

// NewOfficialAccountWebhook 创建公众号回调处理器
func NewOfficialAccountWebhook(
	wxClientFactory *wxInfra.WxClientFactory,
	wxAccountRepo accountPort.WechatAccountRepository,
	mergeLogRepo accountPort.MergeLogRepository,
	appRepo wechatPort.AppRepository,
) *OfficialAccountWebhook {
	return &OfficialAccountWebhook{
		wxClientFactory: wxClientFactory,
		wxAccountRepo:   wxAccountRepo,
		mergeLogRepo:    mergeLogRepo,
		appRepo:         appRepo,
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

	// 2. 处理关注逻辑
	var unionIDPtr *string
	if unionID != "" {
		unionIDPtr = &unionID
	}

	// 2.1 验证微信应用
	app, err := h.appRepo.FindByPlatformAndAppID(ctx, wechatDomain.PlatformOA, appID)
	if err != nil {
		log.Errorw("failed to find wx app", "appID", appID, "error", err)
		return &message.Reply{
			MsgType: message.MsgTypeText,
			MsgData: message.NewText("感谢关注！欢迎使用我们的服务。"),
		}
	}
	if !app.IsEnabled() {
		log.Warnw("wx app is disabled", "appID", appID)
		return &message.Reply{
			MsgType: message.MsgTypeText,
			MsgData: message.NewText("感谢关注！欢迎使用我们的服务。"),
		}
	}

	// 2.2 Upsert wx_accounts
	wxAcc, err := h.wxAccountRepo.FindByOpenID(ctx, appID, accountDomain.WxPlatformOA, openID)
	isNew := false

	if err != nil {
		// 新建账号
		wxAcc, err = accountDomain.NewWechatAccount(int64(app.ID().Value()), appID, accountDomain.WxPlatformOA, openID, unionIDPtr)
		if err != nil {
			log.Errorw("failed to create wx account", "error", err)
			return &message.Reply{
				MsgType: message.MsgTypeText,
				MsgData: message.NewText("感谢关注！欢迎使用我们的服务。"),
			}
		}
		wxAcc.UpdateProfile(nickname, avatar)
		isNew = true
	} else {
		// 更新
		if unionIDPtr != nil && *unionIDPtr != "" {
			wxAcc.UpdateUnionID(*unionIDPtr)
		}
		wxAcc.UpdateProfile(nickname, avatar)
	}

	// 2.3 标记关注
	if err := wxAcc.Follow(); err != nil {
		log.Errorw("failed to follow", "error", err)
		return &message.Reply{
			MsgType: message.MsgTypeText,
			MsgData: message.NewText("感谢关注！欢迎使用我们的服务。"),
		}
	}

	// 2.4 若有 unionid，尝试绑定到已有用户
	if !wxAcc.IsBound() && wxAcc.UnionID() != nil && *wxAcc.UnionID() != "" {
		boundAcc, err := h.wxAccountRepo.FindBoundAccountByUnionID(ctx, *wxAcc.UnionID())
		if err == nil && boundAcc != nil {
			wxAcc.BindUser(*boundAcc.GetUserID())

			// 记录合并日志
			mergeLog := accountDomain.NewMergeLog(*boundAcc.GetUserID(), wxAcc.GetID(), accountDomain.MergeReasonUnionID)
			if err := h.mergeLogRepo.Save(ctx, mergeLog); err != nil {
				log.Errorw("failed to save merge log", "error", err)
			}
		}
	}

	// 2.5 保存/更新
	if isNew {
		err = h.wxAccountRepo.Save(ctx, wxAcc)
	} else {
		err = h.wxAccountRepo.Update(ctx, wxAcc)
	}
	if err != nil {
		log.Errorw("failed to save wx account", "error", err)
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

	// 1. 查找账号
	wxAcc, err := h.wxAccountRepo.FindByOpenID(ctx, appID, accountDomain.WxPlatformOA, openID)
	if err != nil {
		log.Errorw("failed to find wx account", "error", err, "openId", openID)
		return nil
	}

	// 2. 标记取关（不删账号、不解绑用户）
	if err := wxAcc.Unfollow(); err != nil {
		log.Errorw("failed to unfollow", "error", err, "openId", openID)
		return nil
	}

	// 3. 更新
	err = h.wxAccountRepo.Update(ctx, wxAcc)
	if err != nil {
		log.Errorw("failed to update wx account", "error", err, "openId", openID)
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
