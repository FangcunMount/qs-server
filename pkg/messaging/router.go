package messaging

import (
	"context"
	"fmt"
	"sync"
)

// Router 消息路由器
// 用于批量注册消息处理器，支持中间件链
type Router struct {
	subscriber  Subscriber
	handlers    map[string]*handlerConfig
	middlewares []Middleware
	mu          sync.RWMutex
	running     bool
	stopChan    chan struct{}
}

// handlerConfig 处理器配置
type handlerConfig struct {
	topic       string
	channel     string
	handler     Handler
	middlewares []Middleware
}

// NewRouter 创建路由器
func NewRouter(subscriber Subscriber) *Router {
	return &Router{
		subscriber: subscriber,
		handlers:   make(map[string]*handlerConfig),
		stopChan:   make(chan struct{}),
	}
}

// AddHandler 注册消息处理器
// topic: 主题名称
// channel: 通道名称
// handler: 消息处理函数
func (r *Router) AddHandler(topic, channel string, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := fmt.Sprintf("%s:%s", topic, channel)
	r.handlers[key] = &handlerConfig{
		topic:   topic,
		channel: channel,
		handler: handler,
	}
}

// AddHandlerWithMiddleware 注册消息处理器（支持中间件）
// topic: 主题名称
// channel: 通道名称
// handler: 消息处理函数
// middlewares: 中间件列表（按顺序执行）
func (r *Router) AddHandlerWithMiddleware(topic, channel string, handler Handler, middlewares ...Middleware) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := fmt.Sprintf("%s:%s", topic, channel)
	r.handlers[key] = &handlerConfig{
		topic:       topic,
		channel:     channel,
		handler:     handler,
		middlewares: middlewares,
	}
}

// AddMiddleware 添加全局中间件
// 全局中间件会应用到所有处理器
func (r *Router) AddMiddleware(mw Middleware) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.middlewares = append(r.middlewares, mw)
}

// Run 启动路由器
// 将所有注册的处理器订阅到消息中间件
func (r *Router) Run(ctx context.Context) error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return fmt.Errorf("router is already running")
	}
	r.running = true
	r.mu.Unlock()

	// 订阅所有处理器
	for _, cfg := range r.handlers {
		decoratedHandler := r.decorateHandler(cfg.handler, cfg.middlewares)
		if err := r.subscriber.Subscribe(cfg.topic, cfg.channel, decoratedHandler); err != nil {
			return fmt.Errorf("failed to subscribe %s:%s: %w", cfg.topic, cfg.channel, err)
		}
	}

	// 等待退出信号
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.stopChan:
		return nil
	}
}

// Stop 停止路由器
func (r *Router) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return
	}

	r.running = false
	close(r.stopChan)
	r.subscriber.Stop()
}

// decorateHandler 装饰处理器（应用中间件）
// 先应用全局中间件，再应用局部中间件
func (r *Router) decorateHandler(handler Handler, localMiddlewares []Middleware) Handler {
	// 组合所有中间件（全局 + 局部）
	allMiddlewares := append([]Middleware{}, r.middlewares...)
	allMiddlewares = append(allMiddlewares, localMiddlewares...)

	// 从后往前应用中间件（洋葱模型）
	result := handler
	for i := len(allMiddlewares) - 1; i >= 0; i-- {
		result = allMiddlewares[i](result)
	}

	return result
}
