package options

import (
	"errors"
	"testing"

	generic "github.com/FangcunMount/qs-server/internal/pkg/options"
)

func TestTransactionStartupRequiresOfflineMigrationForDomainAndAI(t *testing.T) {
	for _, tc := range []struct {
		name, mode             string
		enabled, missing, want bool
	}{
		{"legacy-auto", "legacy", true, false, false}, {"normal-auto", "normal", true, false, false},
		{"domain-auto", "domain", true, false, true}, {"domain-offline", "domain", false, false, false},
		{"domain-missing", "domain", false, true, true}, {"ai-auto", "ai", true, false, true},
		{"ai-offline", "ai", false, false, false}, {"disabled-ai-config", "disabled-ai", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := NewOptions()
			o.MigrationOptions.Enabled = tc.enabled
			if tc.missing {
				o.MigrationOptions = nil
			}
			mq := &generic.RocketMQOptions{Transactions: map[string]generic.TransactionOptions{"qs.mysql": {StoreID: "primary", WriterEpoch: "m3-1", RouteRevision: "m3-1"}}}
			switch tc.mode {
			case "normal":
				o.MessagingOptions.Provider = "rocketmq"
				o.MessagingOptions.RocketMQ = &generic.RocketMQOptions{}
			case "domain":
				o.MessagingOptions.Provider = "rocketmq"
				o.MessagingOptions.RocketMQ = mq
			case "ai", "disabled-ai":
				o.AIWorkflow.Messaging = AIWorkflowMessagingOptions{Enabled: tc.mode == "ai", Provider: "rocketmq", RocketMQ: mq}
			}
			found := false
			for _, err := range o.Validate() {
				found = found || errors.Is(err, ErrTransactionStartupMigration)
			}
			if found != tc.want {
				t.Fatalf("offline policy error=%v want=%v", found, tc.want)
			}
		})
	}
}
