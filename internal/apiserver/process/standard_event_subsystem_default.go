//go:build !reliable_messaging_m4

package process

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
)

func configuredEventSubsystem(cfg *config.Config) func(eventsubsystem.Options) (*eventsubsystem.Subsystem, error) {
	if cfg != nil && cfg.Eventing != nil && cfg.Eventing.StandardOutbox != nil &&
		(cfg.Eventing.StandardOutbox.Mongo || cfg.Eventing.StandardOutbox.Assessment) {
		return func(eventsubsystem.Options) (*eventsubsystem.Subsystem, error) {
			return nil, fmt.Errorf("standard outbox profile requires the reviewed M4 candidate build")
		}
	}
	return eventsubsystem.New
}
