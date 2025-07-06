package message

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yshujie/questionnaire-scale/pkg/log"
	"github.com/yshujie/questionnaire-scale/pkg/pubsub"
)

// Handler 消息处理器接口
type Handler interface {
	// HandleAnswersheetSaved 处理答卷已保存消息
	HandleAnswersheetSaved(ctx context.Context, message []byte) error
	// GetMessageHandler 获取消息处理器函数
	GetMessageHandler() pubsub.MessageHandler
}

// handler 消息处理器实现
type handler struct {
	// 这里可以注入需要的服务，比如 gRPC 客户端
}

// NewHandler 创建新的消息处理器
func NewHandler() Handler {
	return &handler{}
}

// HandleAnswersheetSaved 处理答卷已保存消息
func (h *handler) HandleAnswersheetSaved(ctx context.Context, message []byte) error {
	log.Infof("Received answersheet saved message: %s", string(message))

	// 解析消息
	var savedMsg pubsub.ResponseSavedMessage
	if err := json.Unmarshal(message, &savedMsg); err != nil {
		return fmt.Errorf("failed to unmarshal answersheet saved message: %w", err)
	}

	log.Infof("Processing answersheet: ResponseID=%s, QuestionnaireID=%s, UserID=%s, SubmittedAt=%d",
		savedMsg.ResponseID, savedMsg.QuestionnaireID, savedMsg.UserID, savedMsg.SubmittedAt)

	// TODO: 实现具体的业务逻辑
	// 1. 通过 gRPC 调用 apiserver 获取答卷详情
	// 2. 通过 gRPC 调用 apiserver 获取问卷和量表信息
	// 3. 执行 scoring 模块（得分计算）
	// 4. 执行 evaluation 模块（报告生成）
	// 5. 通过 gRPC 调用 apiserver 保存解读报告

	log.Infof("Answersheet processing completed for ResponseID: %s", savedMsg.ResponseID)
	return nil
}

// GetMessageHandler 获取消息处理器函数
func (h *handler) GetMessageHandler() pubsub.MessageHandler {
	return func(channel string, message []byte) error {
		ctx := context.Background()

		switch channel {
		case "answersheet.saved":
			return h.HandleAnswersheetSaved(ctx, message)
		default:
			log.Warnf("Unknown message channel: %s", channel)
			return nil
		}
	}
}
