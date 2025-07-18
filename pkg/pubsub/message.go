package pubsub

import (
	"encoding/json"
	"time"
)

// Message 通用消息接口
type Message interface {
	// GetType 获取消息类型
	GetType() string
	// GetSource 获取消息来源
	GetSource() string
	// GetTimestamp 获取消息时间戳
	GetTimestamp() time.Time
	// GetData 获取消息数据
	GetData() interface{}
	// Marshal 序列化消息
	Marshal() ([]byte, error)
}

// BaseMessage 基础消息实现
type BaseMessage struct {
	Type      string      `json:"type"`      // 消息类型
	Source    string      `json:"source"`    // 消息来源
	Timestamp time.Time   `json:"timestamp"` // 消息时间戳
	Data      interface{} `json:"data"`      // 消息数据
}

// NewBaseMessage 创建基础消息
func NewBaseMessage(msgType, source string, data interface{}) *BaseMessage {
	return &BaseMessage{
		Type:      msgType,
		Source:    source,
		Timestamp: time.Now(),
		Data:      data,
	}
}

// GetType 获取消息类型
func (m *BaseMessage) GetType() string {
	return m.Type
}

// GetSource 获取消息来源
func (m *BaseMessage) GetSource() string {
	return m.Source
}

// GetTimestamp 获取消息时间戳
func (m *BaseMessage) GetTimestamp() time.Time {
	return m.Timestamp
}

// GetData 获取消息数据
func (m *BaseMessage) GetData() interface{} {
	return m.Data
}

// Marshal 序列化消息
func (m *BaseMessage) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

// UnmarshalMessage 反序列化消息
func UnmarshalMessage(data []byte) (*BaseMessage, error) {
	var msg BaseMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// MessageHandler 消息处理函数类型 - 使用Message接口
type MessageHandlerV2 func(topic string, msg Message) error

// RawMessageHandler 原始消息处理函数类型 - 兼容现有代码
type RawMessageHandler func(topic string, data []byte) error
