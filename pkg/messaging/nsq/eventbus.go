package nsq

import (
	"fmt"

	"github.com/FangcunMount/qs-server/pkg/messaging"
	"github.com/nsqio/go-nsq"
)

// 在包初始化时注册 NSQ 提供者
func init() {
	messaging.RegisterProvider(messaging.ProviderNSQ, NewEventBusFromConfig)
}

// eventBus NSQ 事件总线实现
type eventBus struct {
	publisher  messaging.Publisher
	subscriber messaging.Subscriber
	router     *messaging.Router
}

// NewEventBus 创建 NSQ 事件总线
// nsqdAddr: NSQd 地址（用于发布）
// lookupdAddrs: NSQLookupd 地址列表（用于订阅）
// cfg: NSQ 配置，如果为 nil 则使用默认配置
func NewEventBus(nsqdAddr string, lookupdAddrs []string, cfg *nsq.Config) (messaging.EventBus, error) {
	if cfg == nil {
		cfg = nsq.NewConfig()
	}

	// 创建发布者
	publisher, err := NewPublisher(nsqdAddr, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create publisher: %w", err)
	}

	// 创建订阅者
	subscriber, err := NewSubscriber(lookupdAddrs, cfg)
	if err != nil {
		publisher.Close()
		return nil, fmt.Errorf("failed to create subscriber: %w", err)
	}

	bus := &eventBus{
		publisher:  publisher,
		subscriber: subscriber,
	}
	bus.router = messaging.NewRouter(subscriber)

	return bus, nil
}

// NewEventBusFromConfig 从配置创建事件总线
func NewEventBusFromConfig(config *messaging.Config) (messaging.EventBus, error) {
	if config == nil {
		config = messaging.DefaultConfig()
	}

	// 创建 NSQ 配置
	nsqCfg := nsq.NewConfig()
	nsqCfg.MaxAttempts = config.NSQ.MaxAttempts
	nsqCfg.MaxInFlight = config.NSQ.MaxInFlight
	nsqCfg.MsgTimeout = config.NSQ.MsgTimeout
	nsqCfg.DefaultRequeueDelay = config.NSQ.RequeueDelay
	nsqCfg.DialTimeout = config.NSQ.DialTimeout
	nsqCfg.ReadTimeout = config.NSQ.ReadTimeout
	nsqCfg.WriteTimeout = config.NSQ.WriteTimeout

	return NewEventBus(config.NSQ.NSQdAddr, config.NSQ.LookupdAddrs, nsqCfg)
}

// Publisher 返回发布者
func (b *eventBus) Publisher() messaging.Publisher {
	return b.publisher
}

// Subscriber 返回订阅者
func (b *eventBus) Subscriber() messaging.Subscriber {
	return b.subscriber
}

// Router 返回路由器
func (b *eventBus) Router() *messaging.Router {
	return b.router
}

// Health 健康检查
func (b *eventBus) Health() error {
	// 尝试 ping NSQ producer
	if p, ok := b.publisher.(*publisher); ok {
		if err := p.producer.Ping(); err != nil {
			return fmt.Errorf("NSQ health check failed: %w", err)
		}
	}
	return nil
}

// Close 关闭事件总线
func (eb *eventBus) Close() error {
	var errs []error

	if err := eb.publisher.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close publisher: %w", err))
	}

	if err := eb.subscriber.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close subscriber: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing eventbus: %v", errs)
	}

	return nil
}
