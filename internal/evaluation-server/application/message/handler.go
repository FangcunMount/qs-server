package message

import (
	"context"
	"fmt"

	internalpubsub "github.com/yshujie/questionnaire-scale/internal/pkg/pubsub"
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
	messageFactory *internalpubsub.MessageFactory
}

// NewHandler 创建消息处理器
func NewHandler() Handler {
	return &handler{
		messageFactory: internalpubsub.NewMessageFactory(),
	}
}

// HandleAnswersheetSaved 处理答卷已保存消息
func (h *handler) HandleAnswersheetSaved(ctx context.Context, message []byte) error {
	log.Infof("Received answersheet saved message: %s", string(message))

	// 使用消息工厂解析消息
	parsedMsg, err := h.messageFactory.ParseMessage(message)
	if err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// 检查消息类型
	if parsedMsg.GetType() != internalpubsub.MessageTypeAnswersheetSaved {
		return fmt.Errorf("unexpected message type: %s", parsedMsg.GetType())
	}

	// 提取答卷数据
	answersheetData, err := internalpubsub.GetAnswersheetSavedData(parsedMsg)
	if err != nil {
		return fmt.Errorf("failed to extract answersheet data: %w", err)
	}

	log.Infof("Processing answersheet: ResponseID=%s, QuestionnaireID=%s, UserID=%s, SubmittedAt=%d",
		answersheetData.ResponseID, answersheetData.QuestionnaireID, answersheetData.UserID, answersheetData.SubmittedAt)

	// TODO: 实现具体的业务逻辑
	// 1. 通过 gRPC 调用 apiserver 获取答卷详情
	// 2. 通过 gRPC 调用 apiserver 获取问卷和量表信息
	// 3. 执行 scoring 模块（得分计算）
	// 4. 执行 evaluation 模块（报告生成）
	// 5. 通过 gRPC 调用 apiserver 保存解读报告

	log.Infof("Answersheet processing completed for ResponseID: %s", answersheetData.ResponseID)
	return nil
}

// GetMessageHandler 获取消息处理器函数
func (h *handler) GetMessageHandler() pubsub.MessageHandler {
	return func(topic string, data []byte) error {
		ctx := context.Background()

		// 根据主题分发消息
		switch topic {
		case "answersheet.saved":
			return h.HandleAnswersheetSaved(ctx, data)
		default:
			log.Warnf("Unknown topic: %s", topic)
			return nil
		}
	}
}
