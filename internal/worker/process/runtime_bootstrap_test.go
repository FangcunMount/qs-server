package process

import (
	"io"
	"log/slog"
	"testing"

	workerconfig "github.com/FangcunMount/qs-server/internal/worker/config"
	workeroptions "github.com/FangcunMount/qs-server/internal/worker/options"
)

func TestWorkerMaxDeliveryAttemptsCompatibilityAndHardCap(t *testing.T) {
	tests := []struct {
		name      string
		raw       map[string]any
		configure func(*workeroptions.Options)
		want      int
	}{
		{name: "new delivery config", raw: map[string]any{"messaging": map[string]any{"delivery": map[string]any{"max-attempts": 7}}}, configure: func(o *workeroptions.Options) { o.Messaging.Delivery.MaxAttempts = 7 }, want: 7},
		{name: "transport retry disabled", raw: map[string]any{"messaging": map[string]any{"delivery": map[string]any{"enable": false}}}, configure: func(o *workeroptions.Options) { o.Messaging.Delivery.Enable = false }, want: 1},
		{name: "legacy config is clamped", raw: map[string]any{"worker": map[string]any{"max-retries": 12}}, configure: func(o *workeroptions.Options) { o.Worker.MaxRetries = 12 }, want: 8},
		{name: "legacy config can lower cap", raw: map[string]any{"worker": map[string]any{"max-retries": 6}}, configure: func(o *workeroptions.Options) { o.Worker.MaxRetries = 6 }, want: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := workeroptions.NewOptions()
			tt.configure(opts)
			if err := opts.ValidateRawSettings(tt.raw); err != nil {
				t.Fatal(err)
			}
			cfg, err := workerconfig.CreateConfigFromOptions(opts)
			if err != nil {
				t.Fatal(err)
			}
			server := &server{config: cfg, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
			if got := server.workerMaxDeliveryAttempts(); got != tt.want {
				t.Fatalf("attempts=%d, want %d", got, tt.want)
			}
		})
	}
}
