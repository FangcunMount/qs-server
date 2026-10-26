package process

import (
	"errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
)

// Validate selection before acquiring resources. Owner constructors still check
// exact original bindings, routes and budgets without installing schema.
func validateFinalMessaging(cfg *config.Config) error {
	retired := errors.New("final RocketMQ transaction owners required; NSQ and publishing Outbox profiles are retired")
	if cfg == nil || cfg.Options == nil || cfg.MessagingOptions == nil || !cfg.MessagingOptions.Enabled || cfg.MessagingOptions.Provider != "rocketmq" || cfg.MessagingOptions.RocketMQ == nil {
		return retired
	}
	owners := cfg.MessagingOptions.RocketMQ.Transactions
	if _, ok := owners["qs.mysql"]; !ok {
		return retired
	}
	if _, ok := owners["qs.mongo"]; !ok || len(owners) != 2 {
		return retired
	}
	if cfg.Eventing != nil && cfg.Eventing.StandardOutbox != nil && (cfg.Eventing.StandardOutbox.Mongo || cfg.Eventing.StandardOutbox.Assessment) {
		return retired
	}
	return nil
}
