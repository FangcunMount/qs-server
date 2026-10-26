package process

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
)

// configuredEventSubsystem constructs only the final shared transaction owners.
func configuredEventSubsystem(cfg *config.Config) func(eventsubsystem.Options) (*eventsubsystem.Subsystem, error) {
	return func(opts eventsubsystem.Options) (*eventsubsystem.Subsystem, error) {
		if err := validateFinalMessaging(cfg); err != nil {
			return nil, err
		}
		return eventsubsystem.NewWithTransactionOwners(opts, cfg.MessagingOptions)
	}
}
