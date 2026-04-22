package notifier

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/FangcunMount/qs-server/internal/worker/port"
)

const webhookSchemaVersion = "v1"
const webhookSignatureAlgorithm = "hmac-sha256"

type webhookEnvelope struct {
	SchemaVersion string    `json:"schema_version"`
	EventID       string    `json:"event_id,omitempty"`
	EventType     string    `json:"event_type"`
	AggregateType string    `json:"aggregate_type,omitempty"`
	AggregateID   string    `json:"aggregate_id,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
	Data          any       `json:"data"`
}

// WebhookNotifier 将任务通知投递到外部 webhook。
type WebhookNotifier struct {
	webhookURL string
	client     *http.Client
	secret     []byte
}

// NewWebhookNotifier 创建 webhook 通知器。
func NewWebhookNotifier(webhookURL string, timeout time.Duration, sharedSecret string) *WebhookNotifier {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &WebhookNotifier{
		webhookURL: webhookURL,
		client: &http.Client{
			Timeout: timeout,
		},
		secret: []byte(sharedSecret),
	}
}

// NotifyTaskCompleted 发送任务完成通知。
func (n *WebhookNotifier) NotifyTaskCompleted(ctx context.Context, meta port.NotificationMeta, payload port.TaskCompletedNotification) error {
	return n.notify(ctx, meta, payload)
}

// NotifyTaskExpired 发送任务过期通知。
func (n *WebhookNotifier) NotifyTaskExpired(ctx context.Context, meta port.NotificationMeta, payload port.TaskExpiredNotification) error {
	return n.notify(ctx, meta, payload)
}

// NotifyTaskCanceled 发送任务取消通知。
func (n *WebhookNotifier) NotifyTaskCanceled(ctx context.Context, meta port.NotificationMeta, payload port.TaskCanceledNotification) error {
	return n.notify(ctx, meta, payload)
}

func (n *WebhookNotifier) notify(ctx context.Context, meta port.NotificationMeta, payload any) error {
	if n == nil || n.webhookURL == "" {
		return nil
	}

	body, err := json.Marshal(webhookEnvelope{
		SchemaVersion: webhookSchemaVersion,
		EventID:       meta.EventID,
		EventType:     meta.EventType,
		AggregateType: meta.AggregateType,
		AggregateID:   meta.AggregateID,
		OccurredAt:    meta.OccurredAt,
		Data:          payload,
	})
	if err != nil {
		return fmt.Errorf("marshal %s notification: %w", meta.EventType, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-QS-Notification-Version", webhookSchemaVersion)
	if meta.EventType != "" {
		req.Header.Set("X-QS-Event-Type", meta.EventType)
	}
	if meta.EventID != "" {
		req.Header.Set("X-QS-Event-ID", meta.EventID)
	}
	if !meta.OccurredAt.IsZero() {
		req.Header.Set("X-QS-Occurred-At", meta.OccurredAt.UTC().Format(time.RFC3339Nano))
	}
	if len(n.secret) > 0 {
		req.Header.Set("X-QS-Signature-Alg", webhookSignatureAlgorithm)
		req.Header.Set("X-QS-Signature", signWebhookPayload(n.secret, body))
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("post webhook notification: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

var _ port.TaskNotifier = (*WebhookNotifier)(nil)

func signWebhookPayload(secret []byte, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
