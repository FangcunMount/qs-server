package nsq

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TopicCreator NSQ Topic 创建器
// 用于在 consumer 启动前预先创建 topic，避免 TOPIC_NOT_FOUND 错误日志
type TopicCreator struct {
	nsqdAddr   string       // NSQd HTTP 地址 (如 localhost:4151)
	httpClient *http.Client // HTTP 客户端
	logger     *slog.Logger
}

// NewTopicCreator 创建 Topic 创建器
// nsqdAddr: NSQd 的 HTTP 地址（注意是 HTTP 端口，通常是 4151，不是 TCP 4150）
func NewTopicCreator(nsqdAddr string, logger *slog.Logger) *TopicCreator {
	// 如果传入的是 TCP 端口 (4150)，自动转换为 HTTP 端口 (4151)
	if strings.HasSuffix(nsqdAddr, ":4150") {
		nsqdAddr = strings.Replace(nsqdAddr, ":4150", ":4151", 1)
	}

	return &TopicCreator{
		nsqdAddr: nsqdAddr,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		logger: logger,
	}
}

// CreateTopic 创建单个 topic
func (t *TopicCreator) CreateTopic(topic string) error {
	endpoint := fmt.Sprintf("http://%s/topic/create?topic=%s", t.nsqdAddr, url.QueryEscape(topic))

	resp, err := t.httpClient.Post(endpoint, "application/json", nil)
	if err != nil {
		return fmt.Errorf("failed to create topic %s: %w", topic, err)
	}
	defer resp.Body.Close()

	// 读取响应体（用于日志）
	body, _ := io.ReadAll(resp.Body)

	// NSQ 返回 200 表示成功（包括 topic 已存在的情况）
	if resp.StatusCode == http.StatusOK {
		t.logger.Debug("Topic created or already exists",
			slog.String("topic", topic),
			slog.Int("status", resp.StatusCode),
		)
		return nil
	}

	return fmt.Errorf("failed to create topic %s: status=%d, body=%s", topic, resp.StatusCode, string(body))
}

// CreateTopics 批量创建 topics
// 返回成功创建的 topic 数量和遇到的错误
func (t *TopicCreator) CreateTopics(topics []string) (int, []error) {
	var errors []error
	successCount := 0

	for _, topic := range topics {
		if err := t.CreateTopic(topic); err != nil {
			errors = append(errors, err)
			t.logger.Warn("Failed to create topic",
				slog.String("topic", topic),
				slog.String("error", err.Error()),
			)
		} else {
			successCount++
		}
	}

	return successCount, errors
}

// EnsureTopics 确保所有 topics 存在
// 这是一个更友好的接口，会记录日志但不会因单个失败而中断
func (t *TopicCreator) EnsureTopics(topics []string) error {
	t.logger.Info("🔧 Creating NSQ topics...",
		slog.Int("count", len(topics)),
		slog.String("nsqd", t.nsqdAddr),
	)

	successCount, errors := t.CreateTopics(topics)

	if len(errors) > 0 {
		t.logger.Warn("⚠️  Some topics failed to create",
			slog.Int("success", successCount),
			slog.Int("failed", len(errors)),
		)
		// 返回第一个错误（可选：返回所有错误的组合）
		return fmt.Errorf("failed to create %d topics, first error: %w", len(errors), errors[0])
	}

	t.logger.Info("✅ All NSQ topics created successfully",
		slog.Int("count", successCount),
	)
	return nil
}

// CreateChannel 创建 channel（可选，channel 会在订阅时自动创建）
func (t *TopicCreator) CreateChannel(topic, channel string) error {
	endpoint := fmt.Sprintf("http://%s/channel/create?topic=%s&channel=%s",
		t.nsqdAddr, url.QueryEscape(topic), url.QueryEscape(channel))

	resp, err := t.httpClient.Post(endpoint, "application/json", nil)
	if err != nil {
		return fmt.Errorf("failed to create channel %s/%s: %w", topic, channel, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.logger.Debug("Channel created or already exists",
			slog.String("topic", topic),
			slog.String("channel", channel),
		)
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("failed to create channel %s/%s: status=%d, body=%s", topic, channel, resp.StatusCode, string(body))
}
