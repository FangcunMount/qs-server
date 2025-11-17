package messaging

import "context"

// ============================================
// 语义化辅助函数：让代码意图更清晰
// ============================================

// PublishEvent 发布事件（语义化别名）
// 用于事件驱动模式，让代码意图更明确
//
// 使用示例：
//
//	messaging.PublishEvent(ctx, publisher, "user.created", data)
func PublishEvent(ctx context.Context, publisher Publisher, event string, body []byte) error {
	return publisher.Publish(ctx, event, body)
}

// PublishTask 发布任务（语义化别名）
// 用于任务队列模式，让代码意图更明确
//
// 使用示例：
//
//	messaging.PublishTask(ctx, publisher, "email.send", task)
func PublishTask(ctx context.Context, publisher Publisher, taskType string, body []byte) error {
	return publisher.Publish(ctx, taskType, body)
}

// SubscribeEvent 订阅事件（事件驱动模式）
// 每个服务应该使用不同的 channel，以便接收所有事件
//
// 使用示例：
//
//	// 每个服务使用唯一的 channel
//	messaging.SubscribeEvent(subscriber, "user.created", "email-service", emailHandler)
//	messaging.SubscribeEvent(subscriber, "user.created", "stat-service", statHandler)
func SubscribeEvent(subscriber Subscriber, event string, serviceName string, handler Handler) error {
	return subscriber.Subscribe(event, serviceName, handler)
}

// SubscribeTask 订阅任务（任务队列模式）
// 同一类型的 worker 应该使用相同的 workerGroup，以实现负载均衡
//
// 使用示例：
//
//	// 多个 worker 使用相同的 workerGroup
//	messaging.SubscribeTask(subscriber, "email.send", "email-workers", handler)
//	messaging.SubscribeTask(subscriber, "email.send", "email-workers", handler)
func SubscribeTask(subscriber Subscriber, taskType string, workerGroup string, handler Handler) error {
	return subscriber.Subscribe(taskType, workerGroup, handler)
}

// ============================================
// 进阶：任务队列辅助类型
// ============================================

// TaskPublisher 任务发布者（语义化包装）
// 这不是新接口，只是对 Publisher 的语义化封装
type TaskPublisher struct {
	publisher Publisher
}

// NewTaskPublisher 创建任务发布者
func NewTaskPublisher(publisher Publisher) *TaskPublisher {
	return &TaskPublisher{publisher: publisher}
}

// Publish 发布任务
func (tp *TaskPublisher) Publish(ctx context.Context, taskType string, body []byte) error {
	return tp.publisher.Publish(ctx, taskType, body)
}

// Close 关闭
func (tp *TaskPublisher) Close() error {
	return tp.publisher.Close()
}

// TaskSubscriber 任务订阅者（语义化包装）
// 这不是新接口，只是对 Subscriber 的语义化封装
type TaskSubscriber struct {
	subscriber  Subscriber
	workerGroup string
}

// NewTaskSubscriber 创建任务订阅者
// workerGroup: 工作组名称，同组的 worker 会负载均衡处理任务
func NewTaskSubscriber(subscriber Subscriber, workerGroup string) *TaskSubscriber {
	return &TaskSubscriber{
		subscriber:  subscriber,
		workerGroup: workerGroup,
	}
}

// Subscribe 订阅任务
func (ts *TaskSubscriber) Subscribe(taskType string, handler Handler) error {
	return ts.subscriber.Subscribe(taskType, ts.workerGroup, handler)
}

// Stop 停止订阅
func (ts *TaskSubscriber) Stop() {
	ts.subscriber.Stop()
}

// Close 关闭
func (ts *TaskSubscriber) Close() error {
	return ts.subscriber.Close()
}

// ============================================
// 使用示例
// ============================================

/*
// 方式1：直接使用底层接口（灵活但需要理解 channel 概念）
subscriber.Subscribe("user.created", "email-service", handler)
subscriber.Subscribe("task.send", "worker-group", handler)

// 方式2：使用语义化函数（推荐：代码意图清晰）
messaging.SubscribeEvent(subscriber, "user.created", "email-service", handler)
messaging.SubscribeTask(subscriber, "task.send", "worker-group", handler)

// 方式3：使用包装类型（可选：适合需要更多封装的场景）
taskSub := messaging.NewTaskSubscriber(subscriber, "worker-group")
taskSub.Subscribe("task.send", handler)
taskSub.Subscribe("task.email", handler)
*/
