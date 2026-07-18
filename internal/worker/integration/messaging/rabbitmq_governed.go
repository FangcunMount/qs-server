package messaging

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"sync"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	amqp "github.com/rabbitmq/amqp091-go"
)

const rabbitDeliveryAttemptHeader = "x-delivery-attempt"

type governedRabbitSubscriber struct {
	conn        *amqp.Connection
	channel     *amqp.Channel
	maxAttempts int
	maxInFlight int
	recorder    DeadLetterRecorder
	consumers   map[string]*rabbitConsumer
	stopCh      chan struct{}
	mu          sync.Mutex
	publishMu   sync.Mutex
}

type rabbitConsumer struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func newGovernedRabbitSubscriber(url string, opts SubscriberOptions) (basemessaging.Subscriber, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("connect rabbitmq: %w", err)
	}
	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open rabbitmq channel: %w", err)
	}
	if opts.MaxInFlight <= 0 {
		opts.MaxInFlight = 1
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 8
	}
	if err := channel.Qos(opts.MaxInFlight, 0, false); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("configure rabbitmq qos: %w", err)
	}
	return &governedRabbitSubscriber{
		conn: conn, channel: channel, maxAttempts: opts.MaxAttempts, maxInFlight: opts.MaxInFlight, recorder: opts.DeadLetters,
		consumers: map[string]*rabbitConsumer{}, stopCh: make(chan struct{}),
	}, nil
}

func (s *governedRabbitSubscriber) Subscribe(topic, channel string, handler basemessaging.Handler) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := topic + ":" + channel
	if _, exists := s.consumers[key]; exists {
		return fmt.Errorf("rabbitmq subscription already exists: %s", key)
	}
	if err := s.declareTopology(topic, channel); err != nil {
		return err
	}
	deliveries, err := s.channel.Consume(channel, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume rabbitmq queue %s: %w", channel, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	consumer := &rabbitConsumer{cancel: cancel, done: make(chan struct{})}
	s.consumers[key] = consumer
	go s.consume(ctx, consumer.done, topic, channel, deliveries, handler)
	return nil
}

func (s *governedRabbitSubscriber) SubscribeWithMiddleware(topic, channel string, handler basemessaging.Handler, middlewares ...basemessaging.Middleware) error {
	for index := len(middlewares) - 1; index >= 0; index-- {
		handler = middlewares[index](handler)
	}
	return s.Subscribe(topic, channel, handler)
}

func (s *governedRabbitSubscriber) declareTopology(topic, channel string) error {
	if err := s.channel.ExchangeDeclare(topic, "fanout", true, false, false, false, nil); err != nil {
		return err
	}
	if err := s.channel.ExchangeDeclare(topic+".retry", "direct", true, false, false, false, nil); err != nil {
		return err
	}
	if err := s.channel.ExchangeDeclare(topic+".dlx", "direct", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := s.channel.QueueDeclare(channel, true, false, false, false, nil); err != nil {
		return err
	}
	if err := s.channel.QueueBind(channel, "", topic, false, nil); err != nil {
		return err
	}
	dlq := channel + ".dlq"
	if _, err := s.channel.QueueDeclare(dlq, true, false, false, false, nil); err != nil {
		return err
	}
	if err := s.channel.QueueBind(dlq, channel, topic+".dlx", false, nil); err != nil {
		return err
	}
	for attempt := 2; attempt <= s.maxAttempts; attempt++ {
		queue := fmt.Sprintf("%s.retry.%d", channel, attempt)
		args := amqp.Table{"x-dead-letter-exchange": topic}
		if _, err := s.channel.QueueDeclare(queue, true, false, false, false, args); err != nil {
			return err
		}
		if err := s.channel.QueueBind(queue, rabbitRetryRoutingKey(channel, attempt), topic+".retry", false, nil); err != nil {
			return err
		}
	}
	return nil
}

func (s *governedRabbitSubscriber) consume(ctx context.Context, done chan struct{}, topic, channel string, deliveries <-chan amqp.Delivery, handler basemessaging.Handler) {
	defer close(done)
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case delivery, ok := <-deliveries:
			if !ok {
				return
			}
			s.handleDelivery(ctx, topic, channel, delivery, handler)
		}
	}
}

func (s *governedRabbitSubscriber) handleDelivery(ctx context.Context, topic, channel string, delivery amqp.Delivery, handler basemessaging.Handler) {
	attempt := rabbitAttempt(delivery.Headers)
	message := &basemessaging.Message{
		UUID: delivery.MessageId, Payload: delivery.Body, Metadata: map[string]string{}, Timestamp: delivery.Timestamp.UnixNano(),
		Topic: topic, Channel: channel, Attempts: uint16(attempt),
	}
	if message.UUID == "" {
		message.UUID = strconv.FormatUint(delivery.DeliveryTag, 10)
	}
	for key, value := range delivery.Headers {
		message.Metadata[key] = fmt.Sprint(value)
	}
	message.SetAckFunc(func() error { return delivery.Ack(false) })
	message.SetNackFunc(func() error { return s.retryOrDeadLetter(ctx, topic, channel, delivery, attempt) })
	if err := handler(ctx, message); err != nil {
		if !message.IsSettled() {
			_ = message.Nack()
		}
		return
	}
	if !message.IsSettled() {
		_ = message.Ack()
	}
}

func (s *governedRabbitSubscriber) retryOrDeadLetter(ctx context.Context, topic, channel string, delivery amqp.Delivery, attempt int) error {
	next := attempt + 1
	exchange := topic + ".retry"
	routingKey := rabbitRetryRoutingKey(channel, next)
	publishing := cloneRabbitPublishing(delivery)
	if attempt >= s.maxAttempts {
		if s.recorder == nil {
			return delivery.Nack(false, true)
		}
		if err := s.recorder.RecordDeadLetter(ctx, deadLetterRecord("rabbitmq", topic, channel, attempt, delivery.MessageId, delivery.Body, "transport delivery exhausted")); err != nil {
			return delivery.Nack(false, true)
		}
		exchange, routingKey = topic+".dlx", channel
		publishing.Headers[rabbitDeliveryAttemptHeader] = int32(attempt)
		delete(publishing.Headers, "expiration")
	} else {
		publishing.Headers[rabbitDeliveryAttemptHeader] = int32(next)
		publishing.Expiration = strconv.FormatInt(transportRetryDelay(attempt, delivery.MessageId).Milliseconds(), 10)
	}
	s.publishMu.Lock()
	err := s.channel.PublishWithContext(ctx, exchange, routingKey, false, false, publishing)
	s.publishMu.Unlock()
	if err != nil {
		return delivery.Nack(false, true)
	}
	return delivery.Ack(false)
}

func cloneRabbitPublishing(delivery amqp.Delivery) amqp.Publishing {
	headers := amqp.Table{}
	for key, value := range delivery.Headers {
		headers[key] = value
	}
	return amqp.Publishing{
		Headers: headers, ContentType: delivery.ContentType, ContentEncoding: delivery.ContentEncoding,
		DeliveryMode: amqp.Persistent, Priority: delivery.Priority, CorrelationId: delivery.CorrelationId,
		ReplyTo: delivery.ReplyTo, MessageId: delivery.MessageId, Timestamp: delivery.Timestamp,
		Type: delivery.Type, UserId: delivery.UserId, AppId: delivery.AppId, Body: delivery.Body,
	}
}

func rabbitAttempt(headers amqp.Table) int {
	if headers == nil {
		return 1
	}
	switch value := headers[rabbitDeliveryAttemptHeader].(type) {
	case int32:
		return int(value)
	case int64:
		return int(value)
	case int:
		return value
	case string:
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 1
}

func rabbitRetryRoutingKey(channel string, attempt int) string {
	return fmt.Sprintf("%s.%d", channel, attempt)
}

func transportRetryDelay(failedAttempt int, messageID string) time.Duration {
	delay := 30 * time.Second
	for step := 1; step < failedAttempt && delay < 5*time.Minute; step++ {
		delay *= 2
		if delay > 5*time.Minute {
			delay = 5 * time.Minute
		}
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(messageID + ":" + strconv.Itoa(failedAttempt)))
	jitterBasisPoints := int(hash.Sum32()%4001) - 2000
	return delay + time.Duration(int64(delay)*int64(jitterBasisPoints)/10000)
}

func (s *governedRabbitSubscriber) Stop() {
	s.mu.Lock()
	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
	}
	consumers := make([]*rabbitConsumer, 0, len(s.consumers))
	for _, consumer := range s.consumers {
		consumer.cancel()
		consumers = append(consumers, consumer)
	}
	s.mu.Unlock()
	for _, consumer := range consumers {
		<-consumer.done
	}
}

func (s *governedRabbitSubscriber) Close() error {
	s.Stop()
	if s.channel != nil {
		_ = s.channel.Close()
	}
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

var _ basemessaging.Subscriber = (*governedRabbitSubscriber)(nil)
