package container

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/worker/options"
)

func TestTaskNotifierConfigurationDoesNotReportEmptyWebhookAsSuccess(t *testing.T) {
	container := &Container{opts: options.NewOptions()}
	if got := container.buildNotifier(); got != nil {
		t.Fatalf("empty notification destination must leave notifier unconfigured, got %T", got)
	}
	container.opts.Notification.WebhookURL = "https://example.test/notify"
	if got := container.buildNotifier(); got == nil {
		t.Fatal("configured webhook notifier missing")
	}
	container.opts.Notification.GatewayURL = "https://example.test/gateway"
	if got := container.buildNotifier(); got == nil {
		t.Fatal("configured gateway notifier missing")
	}
}
