package plan

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/tasktransaction"
)

// TaskOpeningOptions makes the reminder producer choice independent of whether
// an Outbox profile happens to be available. Queued consumers use their own lifecycle.
type TaskOpeningOptions struct {
	ReminderEnabled bool
	ReminderOutbox  eventing.ProfileBinding
	// Explicit selection replaces immediate reminder staging. Failure never
	// falls back to the available legacy Outbox profile. Recovery remains
	// selected when ReminderEnabled is false, so an existing execution cannot
	// be bypassed by disabling new reminder intents.
	Transaction tasktransaction.Opener
}

func (o TaskOpeningOptions) validate() error {
	if o.ReminderEnabled && o.Transaction == nil && o.ReminderOutbox.Stager == nil {
		return missingPlanDependency("enabled task reminder", "MySQL outbox stager")
	}
	return nil
}
