package message

import (
	"context"

	answersheet_saved "github.com/yshujie/questionnaire-scale/internal/evaluation-server/application/message/answersheet-saved"
	grpcclient "github.com/yshujie/questionnaire-scale/internal/evaluation-server/infrastructure/grpc"
	internalpubsub "github.com/yshujie/questionnaire-scale/internal/pkg/pubsub"
	"github.com/yshujie/questionnaire-scale/pkg/log"
	"github.com/yshujie/questionnaire-scale/pkg/pubsub"

	errCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
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
	messageFactory        *internalpubsub.MessageFactory
	answersheetClient     *grpcclient.AnswerSheetClient
	questionnaireClient   *grpcclient.QuestionnaireClient
	medicalScaleClient    *grpcclient.MedicalScaleClient
	interpretReportClient *grpcclient.InterpretReportClient
}

// NewHandler 创建消息处理器
func NewHandler(
	answersheetClient *grpcclient.AnswerSheetClient,
	questionnaireClient *grpcclient.QuestionnaireClient,
	medicalScaleClient *grpcclient.MedicalScaleClient,
	interpretReportClient *grpcclient.InterpretReportClient,
) Handler {
	return &handler{
		messageFactory:        internalpubsub.NewMessageFactory(),
		answersheetClient:     answersheetClient,
		questionnaireClient:   questionnaireClient,
		medicalScaleClient:    medicalScaleClient,
		interpretReportClient: interpretReportClient,
	}
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

// HandleAnswersheetSaved 处理答卷已保存消息
func (h *handler) HandleAnswersheetSaved(ctx context.Context, message []byte) error {
	log.Infof("in HandleAnswersheetSaved: %s", string(message))

	log.Infof("Received answersheet saved message: %s", string(message))

	// 使用消息工厂解析消息
	parsedMsg, err := h.messageFactory.ParseMessage(message)
	if err != nil {
		return errors.WithCode(errCode.ErrInvalidMessage, "failed to parse message")
	}

	// 检查消息类型
	if parsedMsg.GetType() != internalpubsub.MessageTypeAnswersheetSaved {
		return errors.WithCode(errCode.ErrInvalidMessage, "unexpected message type: %s", parsedMsg.GetType())
	}

	// 提取答卷数据
	answersheetSavedData, err := internalpubsub.GetAnswersheetSavedData(parsedMsg)
	if err != nil {
		return errors.WithCode(errCode.ErrInvalidMessage, "failed to extract answersheet data: %w", err)
	}

	// 创建答卷已保存消息处理器链
	answersheetSavedHandlerChain := answersheet_saved.HandlerChain{}

	// 添加计算答卷分数处理器
	answersheetSavedHandlerChain.AddHandler(answersheet_saved.NewCalcAnswersheetScoreHandler(
		h.questionnaireClient,
		h.answersheetClient,
	))

	// 添加创建生成解读报告处理器
	answersheetSavedHandlerChain.AddHandler(answersheet_saved.NewGenerateInterpretReportHandler(
		h.answersheetClient,
		h.medicalScaleClient,
		h.interpretReportClient,
	))

	return answersheetSavedHandlerChain.Handle(ctx, *answersheetSavedData)
}
