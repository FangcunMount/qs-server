package eventconfig

import (
	"sync"
)

// Registry 事件配置注册表（单例）
// 提供全局访问点，避免到处传递配置对象
type Registry struct {
	config *Config
	mu     sync.RWMutex

	// 缓存：事件类型 -> Topic 名称
	eventToTopic map[string]string
	// 缓存：Topic 名称 -> 事件类型列表
	topicToEvents map[string][]string
}

var (
	globalRegistry *Registry
	once           sync.Once
)

// Global 获取全局注册表
func Global() *Registry {
	once.Do(func() {
		globalRegistry = &Registry{
			eventToTopic:  make(map[string]string),
			topicToEvents: make(map[string][]string),
		}
	})
	return globalRegistry
}

// Initialize 初始化全局注册表
func Initialize(configPath string) error {
	cfg, err := Load(configPath)
	if err != nil {
		return err
	}
	Global().SetConfig(cfg)
	return nil
}

// SetConfig 设置配置并构建缓存
func (r *Registry) SetConfig(cfg *Config) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.config = cfg
	r.buildCache()
}

// buildCache 构建查找缓存
func (r *Registry) buildCache() {
	r.eventToTopic = make(map[string]string)
	r.topicToEvents = make(map[string][]string)

	for eventType, eventCfg := range r.config.Events {
		if topicCfg, ok := r.config.Topics[eventCfg.Topic]; ok {
			topicName := topicCfg.Name
			r.eventToTopic[eventType] = topicName
			r.topicToEvents[topicName] = append(r.topicToEvents[topicName], eventType)
		}
	}
}

// Config 获取配置（只读）
func (r *Registry) Config() *Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.config
}

// GetTopicForEvent 获取事件对应的 Topic
func (r *Registry) GetTopicForEvent(eventType string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	topic, ok := r.eventToTopic[eventType]
	return topic, ok
}

// GetEventsForTopic 获取 Topic 下的所有事件
func (r *Registry) GetEventsForTopic(topicName string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.topicToEvents[topicName]
}

// GetTopicConfig 获取 Topic 配置
func (r *Registry) GetTopicConfig(topicKey string) (TopicConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.config == nil {
		return TopicConfig{}, false
	}
	cfg, ok := r.config.Topics[topicKey]
	return cfg, ok
}

// GetEventConfig 获取事件配置
func (r *Registry) GetEventConfig(eventType string) (EventConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.config == nil {
		return EventConfig{}, false
	}
	cfg, ok := r.config.Events[eventType]
	return cfg, ok
}

// AllTopicNames 获取所有 Topic 名称
func (r *Registry) AllTopicNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.topicToEvents))
	for name := range r.topicToEvents {
		names = append(names, name)
	}
	return names
}

// IsEventRegistered 检查事件是否已注册
func (r *Registry) IsEventRegistered(eventType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.eventToTopic[eventType]
	return ok
}
