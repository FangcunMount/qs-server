package aibridge

import (
	opts "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"testing"
)

func TestMQRuntimeRejectsRetiredProviderBeforeDependencies(t *testing.T) {
	for _, provider := range []string{"", "nsq", "other"} {
		t.Run(provider, func(t *testing.T) {
			_, err := NewMessagingRuntime(opts.AIWorkflowMessagingOptions{Enabled: true, Provider: provider}, nil, nil, nil)
			if err == nil || err.Error() != "AI runtime requires RocketMQ transaction owners" {
				t.Fatalf("retired configuration reached dependencies: %v", err)
			}
		})
	}
}
