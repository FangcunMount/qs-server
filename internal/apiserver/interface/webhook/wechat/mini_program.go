package wechat

import (
	"context"

	"github.com/gin-gonic/gin"

	wxInfra "github.com/fangcun-mount/qs-server/internal/apiserver/infra/wechat"
	"github.com/fangcun-mount/qs-server/pkg/core"
	"github.com/fangcun-mount/qs-server/pkg/log"
)

// MiniProgramWebhook 小程序回调处理器
type MiniProgramWebhook struct {
	wxClientFactory *wxInfra.WxClientFactory
}

// NewMiniProgramWebhook 创建小程序回调处理器
func NewMiniProgramWebhook(wxClientFactory *wxInfra.WxClientFactory) *MiniProgramWebhook {
	return &MiniProgramWebhook{
		wxClientFactory: wxClientFactory,
	}
}

// HandleCallback 处理小程序回调
// POST /webhook/wechat/mp/:appId
// 注：小程序回调较少使用，主要用于消息推送、订阅通知等
func (h *MiniProgramWebhook) HandleCallback(c *gin.Context) {
	appID := c.Param("appId")

	log.Infow("received miniprogram callback",
		"appId", appID,
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
	)

	// 1. 验证签名（可选）
	signature := c.Query("signature")
	timestamp := c.Query("timestamp")
	nonce := c.Query("nonce")

	log.Infow("callback params",
		"signature", signature,
		"timestamp", timestamp,
		"nonce", nonce,
	)

	// 2. 解析消息体
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		log.Errorw("failed to parse callback body", "error", err)
		core.WriteResponse(c, err, nil)
		return
	}

	log.Infow("callback body", "body", body)

	// 3. 根据消息类型处理
	msgType, ok := body["MsgType"].(string)
	if !ok {
		log.Warnw("missing MsgType in callback")
		core.WriteResponse(c, nil, gin.H{"errcode": 0, "errmsg": "ok"})
		return
	}

	ctx := c.Request.Context()
	switch msgType {
	case "event":
		h.handleEvent(ctx, appID, body)
	case "text":
		h.handleTextMessage(ctx, appID, body)
	default:
		log.Infow("unsupported message type", "msgType", msgType)
	}

	// 4. 返回成功响应
	core.WriteResponse(c, nil, gin.H{"errcode": 0, "errmsg": "ok"})
}

// handleEvent 处理事件
func (h *MiniProgramWebhook) handleEvent(ctx context.Context, appID string, body map[string]interface{}) {
	event, ok := body["Event"].(string)
	if !ok {
		log.Warnw("missing Event in callback")
		return
	}

	log.Infow("received miniprogram event",
		"appId", appID,
		"event", event,
		"body", body,
	)

	// TODO: 根据业务需求处理不同的事件
	// 例如：用户进入小程序、订阅消息状态变更等
}

// handleTextMessage 处理文本消息
func (h *MiniProgramWebhook) handleTextMessage(ctx context.Context, appID string, body map[string]interface{}) {
	content, _ := body["Content"].(string)
	fromUser, _ := body["FromUserName"].(string)

	log.Infow("received miniprogram text message",
		"appId", appID,
		"fromUser", fromUser,
		"content", content,
	)

	// TODO: 处理文本消息
	// 注：小程序客服消息需要在48小时内回复
}
