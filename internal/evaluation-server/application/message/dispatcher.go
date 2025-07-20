package message

import (
	"context"
	"fmt"

	internalpubsub "github.com/yshujie/questionnaire-scale/internal/pkg/pubsub"
	"github.com/yshujie/questionnaire-scale/pkg/log"
	"github.com/yshujie/questionnaire-scale/pkg/pubsub"

	errCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// MessageProcessor 消息处理器接口
type MessageProcessor interface {
	Process(ctx context.Context, data []byte) error
	GetMessageType() string
}

// MessageDispatcher 消息分发器
type MessageDispatcher struct {
	messageFactory *internalpubsub.MessageFactory
	processors     map[string]MessageProcessor
}

// NewMessageDispatcher 创建消息分发器
func NewMessageDispatcher() *MessageDispatcher {
	return &MessageDispatcher{
		messageFactory: internalpubsub.NewMessageFactory(),
		processors:     make(map[string]MessageProcessor),
	}
}

// RegisterProcessor 注册消息处理器
func (d *MessageDispatcher) RegisterProcessor(processor MessageProcessor) {
	d.processors[processor.GetMessageType()] = processor
}

// GetMessageHandler 获取消息处理器函数
func (d *MessageDispatcher) GetMessageHandler() pubsub.MessageHandler {
	return func(topic string, data []byte) error {
		ctx := context.Background()
		return d.DispatchMessage(ctx, topic, data)
	}
}

// DispatchMessage 分发消息
func (d *MessageDispatcher) DispatchMessage(ctx context.Context, topic string, data []byte) error {
	log.Infof("Received message from topic: %s", topic)

	// 解析消息
	parsedMsg, err := d.messageFactory.ParseMessage(data)
	if err != nil {
		return errors.WithCode(errCode.ErrInvalidMessage, "failed to parse message: %w", err)
	}

	// 获取消息类型
	messageType := parsedMsg.GetType()
	log.Infof("Message type: %s", messageType)

	// 查找对应的处理器
	processor, exists := d.processors[messageType]
	if !exists {
		log.Warnf("No processor found for message type: %s", messageType)
		return nil
	}

	// 处理消息
	return processor.Process(ctx, data)
}

// AnswersheetSavedProcessor 答卷保存消息处理器
type AnswersheetSavedProcessor struct {
	handlerChain *AnswersheetSavedHandlerChain
}

// NewAnswersheetSavedProcessor 创建答卷保存消息处理器
func NewAnswersheetSavedProcessor(handlerChain *AnswersheetSavedHandlerChain) *AnswersheetSavedProcessor {
	return &AnswersheetSavedProcessor{
		handlerChain: handlerChain,
	}
}

// Process 处理答卷保存消息
func (p *AnswersheetSavedProcessor) Process(ctx context.Context, data []byte) error {
	log.Infof("Processing answersheet saved message: %s", string(data))

	// 解析消息
	messageFactory := internalpubsub.NewMessageFactory()
	parsedMsg, err := messageFactory.ParseMessage(data)
	if err != nil {
		return errors.WithCode(errCode.ErrInvalidMessage, "failed to parse message: %w", err)
	}

	// 提取答卷数据
	answersheetSavedData, err := internalpubsub.GetAnswersheetSavedData(parsedMsg)
	if err != nil {
		return errors.WithCode(errCode.ErrInvalidMessage, "failed to extract answersheet data: %w", err)
	}

	// 使用处理器链处理
	return p.handlerChain.Handle(ctx, *answersheetSavedData)
}

// GetMessageType 获取消息类型
func (p *AnswersheetSavedProcessor) GetMessageType() string {
	return internalpubsub.MessageTypeAnswersheetSaved
}

// AnswersheetSavedHandlerChain 答卷保存处理器链
type AnswersheetSavedHandlerChain struct {
	handlers []AnswersheetSavedHandler
}

// AnswersheetSavedHandler 答卷保存处理器接口
type AnswersheetSavedHandler interface {
	Handle(ctx context.Context, data internalpubsub.AnswersheetSavedData) error
}

// AddHandler 添加处理器
func (c *AnswersheetSavedHandlerChain) AddHandler(handler AnswersheetSavedHandler) {
	c.handlers = append(c.handlers, handler)
}

// Handle 处理答卷保存事件
func (c *AnswersheetSavedHandlerChain) Handle(ctx context.Context, data internalpubsub.AnswersheetSavedData) error {
	for _, handler := range c.handlers {
		if err := handler.Handle(ctx, data); err != nil {
			return fmt.Errorf("handler failed: %w", err)
		}
	}
	return nil
}
