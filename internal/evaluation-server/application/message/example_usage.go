package message

import (
	"context"
	"fmt"

	grpcclient "github.com/yshujie/questionnaire-scale/internal/evaluation-server/infrastructure/grpc"
	internalpubsub "github.com/yshujie/questionnaire-scale/internal/pkg/pubsub"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// ExampleUsage 演示如何使用新的消息处理架构
func ExampleUsage() {
	log.Info("=== 演示新的消息处理架构 ===")

	// 1. 创建 gRPC 客户端（实际使用中从容器获取）
	// 这里只是演示，实际的客户端创建会更复杂
	var (
		questionnaireClient   *grpcclient.QuestionnaireClient
		answersheetClient     *grpcclient.AnswerSheetClient
		medicalScaleClient    *grpcclient.MedicalScaleClient
		interpretReportClient *grpcclient.InterpretReportClient
	)

	// 2. 使用新的架构创建消息处理器
	handler := NewHandler(
		answersheetClient,
		questionnaireClient,
		medicalScaleClient,
		interpretReportClient,
	)

	// 3. 获取消息处理函数
	messageHandler := handler.GetMessageHandler()

	// 4. 模拟接收到消息
	topic := "answersheet.saved"
	messageData := []byte(`{
		"type": "answersheet_saved",
		"data": {
			"answer_sheet_id": 123,
			"questionnaire_code": "PHQ-9",
			"questionnaire_version": "1.0"
		}
	}`)

	// 5. 处理消息
	if err := messageHandler(topic, messageData); err != nil {
		log.Errorf("消息处理失败: %v", err)
	} else {
		log.Info("消息处理成功")
	}

	log.Info("=== 演示完成 ===")
}

// ExampleCustomProcessor 演示如何添加自定义消息处理器
func ExampleCustomProcessor() {
	log.Info("=== 演示添加自定义消息处理器 ===")

	// 1. 创建消息分发器
	dispatcher := NewMessageDispatcher()

	// 2. 创建自定义消息处理器
	customProcessor := &CustomMessageProcessor{}

	// 3. 注册自定义处理器
	dispatcher.RegisterProcessor(customProcessor)

	// 4. 处理自定义消息
	ctx := context.Background()
	customMessage := []byte(`{
		"type": "custom_message",
		"data": {
			"custom_field": "custom_value"
		}
	}`)

	if err := dispatcher.DispatchMessage(ctx, "custom.topic", customMessage); err != nil {
		log.Errorf("自定义消息处理失败: %v", err)
	} else {
		log.Info("自定义消息处理成功")
	}

	log.Info("=== 自定义处理器演示完成 ===")
}

// CustomMessageProcessor 自定义消息处理器示例
type CustomMessageProcessor struct{}

// Process 处理自定义消息
func (p *CustomMessageProcessor) Process(ctx context.Context, data []byte) error {
	log.Infof("处理自定义消息: %s", string(data))

	// 这里可以实现具体的业务逻辑
	// 例如：解析消息、调用服务、保存数据等

	return nil
}

// GetMessageType 获取消息类型
func (p *CustomMessageProcessor) GetMessageType() string {
	return "custom_message"
}

// ExampleExtendingHandlerChain 演示如何扩展处理器链
func ExampleExtendingHandlerChain() {
	log.Info("=== 演示扩展处理器链 ===")

	// 1. 创建处理器链
	handlerChain := &AnswersheetSavedHandlerChain{}

	// 2. 添加现有的处理器
	// handlerChain.AddHandler(existingHandler1)
	// handlerChain.AddHandler(existingHandler2)

	// 3. 添加自定义处理器
	customHandler := &CustomAnswersheetSavedHandler{}
	handlerChain.AddHandler(customHandler)

	// 4. 使用处理器链
	ctx := context.Background()
	data := internalpubsub.AnswersheetSavedData{
		AnswerSheetID:        123,
		QuestionnaireCode:    "PHQ-9",
		QuestionnaireVersion: "1.0",
	}

	if err := handlerChain.Handle(ctx, data); err != nil {
		log.Errorf("处理器链执行失败: %v", err)
	} else {
		log.Info("处理器链执行成功")
	}

	log.Info("=== 处理器链扩展演示完成 ===")
}

// CustomAnswersheetSavedHandler 自定义答卷保存处理器
type CustomAnswersheetSavedHandler struct{}

// Handle 处理答卷保存事件
func (h *CustomAnswersheetSavedHandler) Handle(ctx context.Context, data internalpubsub.AnswersheetSavedData) error {
	log.Infof("自定义处理器处理答卷保存事件: AnswerSheetID=%d, QuestionnaireCode=%s",
		data.AnswerSheetID, data.QuestionnaireCode)

	// 这里可以实现自定义的业务逻辑
	// 例如：发送通知、更新缓存、记录审计日志等

	return nil
}

// ArchitectureBenefits 展示新架构的优势
func ArchitectureBenefits() {
	fmt.Println(`
🎯 新消息处理架构的优势:

1. 📦 单一职责原则 (SRP)
   - MessageDispatcher: 专门负责消息分发
   - MessageProcessor: 专门负责特定类型消息处理
   - HandlerChain: 专门负责业务处理流程

2. 🔓 开闭原则 (OCP)
   - 添加新消息类型: 只需实现 MessageProcessor 接口
   - 添加新处理步骤: 只需实现 AnswersheetSavedHandler 接口
   - 无需修改现有代码

3. 🔄 依赖倒置原则 (DIP)
   - 依赖抽象接口而不是具体实现
   - 便于单元测试和Mock

4. 🔧 可扩展性
   - 支持多种消息类型
   - 支持动态注册处理器
   - 支持处理器链模式

5. 🧪 可测试性
   - 每个组件都可以独立测试
   - 便于Mock依赖

6. 📈 可维护性
   - 代码结构清晰
   - 职责分离明确
   - 修改影响范围小

使用示例:
  handler := NewHandler(clients...)
  messageHandler := handler.GetMessageHandler()
  messageHandler("topic", messageData)
`)
}
