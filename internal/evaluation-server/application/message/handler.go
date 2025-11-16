package message

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	answersheet_saved "github.com/FangcunMount/qs-server/internal/evaluation-server/application/message/answersheet-saved"
	"github.com/FangcunMount/qs-server/internal/evaluation-server/application/repository"
	grpcclient "github.com/FangcunMount/qs-server/internal/evaluation-server/infrastructure/grpc"
	"github.com/FangcunMount/qs-server/pkg/pubsub"
)

// Handler 消息处理器接口（保持向后兼容）
type Handler interface {
	// HandleAnswersheetSaved 处理答卷已保存消息
	HandleAnswersheetSaved(ctx context.Context, message []byte) error
	// GetMessageHandler 获取消息处理器函数
	GetMessageHandler() pubsub.MessageHandler
}

// handler 消息处理器实现（使用新的dispatcher架构）
type handler struct {
	dispatcher *MessageDispatcher
}

// NewHandler 创建消息处理器
func NewHandler(
	answersheetClient *grpcclient.AnswerSheetClient,
	questionnaireClient *grpcclient.QuestionnaireClient,
	medicalScaleClient *grpcclient.MedicalScaleClient,
	interpretReportClient *grpcclient.InterpretReportClient,
) Handler {
	// 创建消息分发器
	dispatcher := NewMessageDispatcher()

	// 创建答卷保存处理器链
	handlerChain := &AnswersheetSavedHandlerChain{}

	// 添加计算答卷分数处理器
	handlerChain.AddHandler(answersheet_saved.NewCalcAnswersheetScoreHandler(
		questionnaireClient,
		answersheetClient,
	))

	// 添加生成解读报告处理器（使用并发版本）
	handlerChain.AddHandler(answersheet_saved.NewGenerateInterpretReportHandlerConcurrent(
		answersheetClient,
		medicalScaleClient,
		interpretReportClient,
		10, // 设置最大并发数为10
	))

	// 创建答卷保存消息处理器
	answersheetProcessor := NewAnswersheetSavedProcessor(handlerChain)

	// 注册处理器到分发器
	dispatcher.RegisterProcessor(answersheetProcessor)

	return &handler{
		dispatcher: dispatcher,
	}
}

// GetMessageHandler 获取消息处理器函数
func (h *handler) GetMessageHandler() pubsub.MessageHandler {
	return h.dispatcher.GetMessageHandler()
}

// HandleAnswersheetSaved 处理答卷已保存消息（保持向后兼容）
func (h *handler) HandleAnswersheetSaved(ctx context.Context, message []byte) error {
	log.Infof("HandleAnswersheetSaved called (legacy method): %s", string(message))

	// 使用新的分发器处理
	return h.dispatcher.DispatchMessage(ctx, "answersheet.saved", message)
}

// NewHandlerWithRepositories 使用仓储接口创建消息处理器（推荐的新方式）
func NewHandlerWithRepositories(
	questionnaireRepo repository.QuestionnaireRepository,
	answerSheetRepo repository.AnswerSheetRepository,
	medicalScaleRepo repository.MedicalScaleRepository,
	interpretReportRepo repository.InterpretReportRepository,
) Handler {
	// 创建消息分发器
	dispatcher := NewMessageDispatcher()

	// 创建答卷保存处理器链
	handlerChain := &AnswersheetSavedHandlerChain{}

	// 使用仓储接口创建处理器（这里需要适配器模式）
	// 注意：这里为了演示依赖抽象的概念，实际使用中需要创建适配器
	// handlerChain.AddHandler(NewScoreCalculationHandler(questionnaireRepo, answerSheetRepo))
	// handlerChain.AddHandler(NewReportGenerationHandler(answerSheetRepo, medicalScaleRepo, interpretReportRepo))

	// 创建答卷保存消息处理器
	answersheetProcessor := NewAnswersheetSavedProcessor(handlerChain)

	// 注册处理器到分发器
	dispatcher.RegisterProcessor(answersheetProcessor)

	return &handler{
		dispatcher: dispatcher,
	}
}

// NewHandlerWithConcurrency 创建消息处理器（支持配置并发数）
func NewHandlerWithConcurrency(
	answersheetClient *grpcclient.AnswerSheetClient,
	questionnaireClient *grpcclient.QuestionnaireClient,
	medicalScaleClient *grpcclient.MedicalScaleClient,
	interpretReportClient *grpcclient.InterpretReportClient,
	maxConcurrency int,
) Handler {
	// 创建消息分发器
	dispatcher := NewMessageDispatcher()

	// 创建答卷保存处理器链
	handlerChain := &AnswersheetSavedHandlerChain{}

	// 添加计算答卷分数处理器
	handlerChain.AddHandler(answersheet_saved.NewCalcAnswersheetScoreHandler(
		questionnaireClient,
		answersheetClient,
	))

	// 添加生成解读报告处理器（使用并发版本，可配置并发数）
	handlerChain.AddHandler(answersheet_saved.NewGenerateInterpretReportHandlerConcurrent(
		answersheetClient,
		medicalScaleClient,
		interpretReportClient,
		maxConcurrency,
	))

	// 创建答卷保存消息处理器
	answersheetProcessor := NewAnswersheetSavedProcessor(handlerChain)

	// 注册处理器到分发器
	dispatcher.RegisterProcessor(answersheetProcessor)

	return &handler{
		dispatcher: dispatcher,
	}
}
