package answersheet_saved

import (
	"context"

	grpcclient "github.com/yshujie/questionnaire-scale/internal/evaluation-server/infrastructure/grpc"
	internalpubsub "github.com/yshujie/questionnaire-scale/internal/pkg/pubsub"
)

// HandlerAdapter 适配器，将现有的Handler适配到新的AnswersheetSavedHandler接口
type HandlerAdapter struct {
	handler Handler // 现有的Handler接口
}

// NewHandlerAdapter 创建处理器适配器
func NewHandlerAdapter(handler Handler) *HandlerAdapter {
	return &HandlerAdapter{
		handler: handler,
	}
}

// Handle 实现新的AnswersheetSavedHandler接口
func (a *HandlerAdapter) Handle(ctx context.Context, data internalpubsub.AnswersheetSavedData) error {
	return a.handler.Handle(ctx, data)
}

// CalcAnswersheetScoreHandlerAdapter 计算答卷分数处理器适配器
type CalcAnswersheetScoreHandlerAdapter struct {
	*CalcAnswersheetScoreHandler
}

// NewCalcAnswersheetScoreHandlerAdapter 创建计算答卷分数处理器适配器
func NewCalcAnswersheetScoreHandlerAdapter(
	questionnaireClient *grpcclient.QuestionnaireClient,
	answersheetClient *grpcclient.AnswerSheetClient,
) *CalcAnswersheetScoreHandlerAdapter {
	return &CalcAnswersheetScoreHandlerAdapter{
		CalcAnswersheetScoreHandler: NewCalcAnswersheetScoreHandler(questionnaireClient, answersheetClient),
	}
}

// Handle 实现新的AnswersheetSavedHandler接口
func (a *CalcAnswersheetScoreHandlerAdapter) Handle(ctx context.Context, data internalpubsub.AnswersheetSavedData) error {
	return a.CalcAnswersheetScoreHandler.Handle(ctx, data)
}

// GenerateInterpretReportHandlerAdapter 生成解读报告处理器适配器
type GenerateInterpretReportHandlerAdapter struct {
	*GenerateInterpretReportHandler
}

// NewGenerateInterpretReportHandlerAdapter 创建生成解读报告处理器适配器
func NewGenerateInterpretReportHandlerAdapter(
	answersheetClient *grpcclient.AnswerSheetClient,
	medicalScaleClient *grpcclient.MedicalScaleClient,
	interpretReportClient *grpcclient.InterpretReportClient,
) *GenerateInterpretReportHandlerAdapter {
	return &GenerateInterpretReportHandlerAdapter{
		GenerateInterpretReportHandler: NewGenerateInterpretReportHandler(
			answersheetClient,
			medicalScaleClient,
			interpretReportClient,
		),
	}
}

// Handle 实现新的AnswersheetSavedHandler接口
func (a *GenerateInterpretReportHandlerAdapter) Handle(ctx context.Context, data internalpubsub.AnswersheetSavedData) error {
	return a.GenerateInterpretReportHandler.Handle(ctx, data)
}
