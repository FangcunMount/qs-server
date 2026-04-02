package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/FangcunMount/qs-server/internal/worker/port"
)

const gatewaySchemaVersion = "v1"

type gatewayEnvelope struct {
	SchemaVersion    string                `json:"schema_version"`
	NotificationType string                `json:"notification_type"`
	TemplateCode     string                `json:"template_code"`
	Event            port.NotificationMeta `json:"event"`
	Recipient        gatewayRecipient      `json:"recipient"`
	Data             any                   `json:"data"`
}

type gatewayRecipient struct {
	TesteeID string `json:"testee_id"`
}

// GatewayNotifier 将任务通知发送到内部通知网关，由网关决定具体渠道。
type GatewayNotifier struct {
	gatewayURL string
	authToken  string
	client     *http.Client
}

func NewGatewayNotifier(gatewayURL, authToken string, timeout time.Duration) *GatewayNotifier {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if gatewayURL == "" {
		return nil
	}
	return &GatewayNotifier{
		gatewayURL: gatewayURL,
		authToken:  authToken,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (n *GatewayNotifier) NotifyTaskCompleted(ctx context.Context, meta port.NotificationMeta, payload port.TaskCompletedNotification) error {
	return n.notify(ctx, gatewayEnvelope{
		SchemaVersion:    gatewaySchemaVersion,
		NotificationType: meta.EventType,
		TemplateCode:     "plan_task_completed",
		Event:            meta,
		Recipient: gatewayRecipient{
			TesteeID: payload.TesteeID,
		},
		Data: payload,
	})
}

func (n *GatewayNotifier) NotifyTaskExpired(ctx context.Context, meta port.NotificationMeta, payload port.TaskExpiredNotification) error {
	return n.notify(ctx, gatewayEnvelope{
		SchemaVersion:    gatewaySchemaVersion,
		NotificationType: meta.EventType,
		TemplateCode:     "plan_task_expired",
		Event:            meta,
		Recipient: gatewayRecipient{
			TesteeID: payload.TesteeID,
		},
		Data: payload,
	})
}

func (n *GatewayNotifier) NotifyTaskCanceled(ctx context.Context, meta port.NotificationMeta, payload port.TaskCanceledNotification) error {
	return n.notify(ctx, gatewayEnvelope{
		SchemaVersion:    gatewaySchemaVersion,
		NotificationType: meta.EventType,
		TemplateCode:     "plan_task_canceled",
		Event:            meta,
		Recipient: gatewayRecipient{
			TesteeID: payload.TesteeID,
		},
		Data: payload,
	})
}

func (n *GatewayNotifier) notify(ctx context.Context, payload gatewayEnvelope) error {
	if n == nil || n.gatewayURL == "" {
		return nil
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal gateway notification: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.gatewayURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build gateway request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-QS-Notification-Version", gatewaySchemaVersion)
	if payload.NotificationType != "" {
		req.Header.Set("X-QS-Event-Type", payload.NotificationType)
	}
	if payload.Event.EventID != "" {
		req.Header.Set("X-QS-Event-ID", payload.Event.EventID)
	}
	if n.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+n.authToken)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("post gateway notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("gateway returned status %d", resp.StatusCode)
	}

	return nil
}

var _ port.TaskNotifier = (*GatewayNotifier)(nil)
