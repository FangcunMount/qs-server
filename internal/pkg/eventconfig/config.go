// Package eventconfig 提供事件配置的加载和解析
//
// 配置驱动的事件系统：
// - 事件类型、Topic 映射、处理器配置统一在 YAML 中定义
// - 发布端根据配置路由事件到正确的 Topic
// - 订阅端根据配置自动注册 Topic 和处理器
package eventconfig

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 事件配置根结构
type Config struct {
	Version  string                 `yaml:"version"`
	Topics   map[string]TopicConfig `yaml:"topics"`
	Events   map[string]EventConfig `yaml:"events"`
	Handlers map[string]HandlerMeta `yaml:"handlers"`
}

// TopicConfig Topic 配置
type TopicConfig struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Consumer    ConsumerConfig `yaml:"consumer"`
}

// ConsumerConfig 消费者配置
type ConsumerConfig struct {
	Group       string      `yaml:"group"`
	Concurrency int         `yaml:"concurrency"`
	Retry       RetryConfig `yaml:"retry"`
}

// RetryConfig 重试配置
type RetryConfig struct {
	MaxAttempts     int           `yaml:"max_attempts"`
	InitialInterval time.Duration `yaml:"initial_interval"`
	MaxInterval     time.Duration `yaml:"max_interval"`
}

// EventConfig 事件配置
type EventConfig struct {
	Topic       string   `yaml:"topic"`       // Topic 引用（对应 Topics 中的 key）
	Aggregate   string   `yaml:"aggregate"`   // 聚合类型
	Domain      string   `yaml:"domain"`      // 所属领域
	Description string   `yaml:"description"` // 事件描述
	Handler     string   `yaml:"handler"`     // 处理器引用
	Consumers   []string `yaml:"consumers"`   // 消费者列表
	Priority    string   `yaml:"priority"`    // 优先级（high, normal, low）
}

// HandlerMeta 处理器元信息
type HandlerMeta struct {
	Package     string `yaml:"package"`     // 所在包
	Description string `yaml:"description"` // 描述
}

// Load 从文件加载事件配置
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return Parse(data)
}

// Parse 解析事件配置
func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// Validate 验证配置完整性
func (c *Config) Validate() error {
	// 验证事件引用的 Topic 存在
	for eventType, eventCfg := range c.Events {
		if _, ok := c.Topics[eventCfg.Topic]; !ok {
			return fmt.Errorf("event %q references unknown topic %q", eventType, eventCfg.Topic)
		}
		if eventCfg.Handler != "" {
			if _, ok := c.Handlers[eventCfg.Handler]; !ok {
				return fmt.Errorf("event %q references unknown handler %q", eventType, eventCfg.Handler)
			}
		}
	}
	return nil
}

// GetTopicName 获取事件对应的 Topic 名称
func (c *Config) GetTopicName(eventType string) (string, bool) {
	eventCfg, ok := c.Events[eventType]
	if !ok {
		return "", false
	}
	topicCfg, ok := c.Topics[eventCfg.Topic]
	if !ok {
		return "", false
	}
	return topicCfg.Name, true
}

// GetEventsByTopic 获取 Topic 下的所有事件类型
func (c *Config) GetEventsByTopic(topicKey string) []string {
	var events []string
	for eventType, eventCfg := range c.Events {
		if eventCfg.Topic == topicKey {
			events = append(events, eventType)
		}
	}
	return events
}

// GetTopicKeys 获取所有 Topic 的 key
func (c *Config) GetTopicKeys() []string {
	keys := make([]string, 0, len(c.Topics))
	for k := range c.Topics {
		keys = append(keys, k)
	}
	return keys
}

// GetHandlerName 获取事件对应的处理器名称
func (c *Config) GetHandlerName(eventType string) (string, bool) {
	eventCfg, ok := c.Events[eventType]
	if !ok {
		return "", false
	}
	return eventCfg.Handler, eventCfg.Handler != ""
}

// ListEventTypes 列出所有事件类型
func (c *Config) ListEventTypes() []string {
	types := make([]string, 0, len(c.Events))
	for t := range c.Events {
		types = append(types, t)
	}
	return types
}
