package messaging

import (
	"context"
	"encoding/json"
)

// Helper 提供便捷的消息发布方法

// PublishJSON 发布 JSON 消息
// 将对象序列化为 JSON 后发布
func PublishJSON(ctx context.Context, publisher Publisher, topic string, data interface{}) error {
	body, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return publisher.Publish(ctx, topic, body)
}

// ParseJSON 解析 JSON 消息
// 将消息体解析为指定类型
func ParseJSON(msg *Message, v interface{}) error {
	return json.Unmarshal(msg.Payload, v)
}

// WrapHandler 包装旧版本的 handler
// 用于兼容旧的 func(topic string, data []byte) error 签名
func WrapHandler(oldHandler func(topic string, data []byte) error) Handler {
	return func(ctx context.Context, msg *Message) error {
		return oldHandler(msg.Topic, msg.Payload)
	}
}

// NewSimpleHandler 创建简单的处理器
// 只关心消息体，不关心其他元数据
func NewSimpleHandler(fn func([]byte) error) Handler {
	return func(ctx context.Context, msg *Message) error {
		return fn(msg.Payload)
	}
}

// NewJSONHandler 创建 JSON 处理器
// 自动解析 JSON 消息并调用业务处理函数
func NewJSONHandler(fn func(ctx context.Context, data interface{}) error, dataType interface{}) Handler {
	return func(ctx context.Context, msg *Message) error {
		// 创建新的实例
		v := dataType
		if err := json.Unmarshal(msg.Payload, &v); err != nil {
			return err
		}
		return fn(ctx, v)
	}
}
