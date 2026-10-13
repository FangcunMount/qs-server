package plan

import "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"

// TaskOpeningOptions makes the reminder producer choice independent of whether
// an Outbox profile happens to be available. Queued consumers use their own lifecycle.
type TaskOpeningOptions struct {
	ReminderEnabled bool
	ReminderOutbox  eventing.ProfileBinding
}

func (o TaskOpeningOptions) validate() error {
	if o.ReminderEnabled && o.ReminderOutbox.Stager == nil {
		return missingPlanDependency("enabled task reminder", "MySQL outbox stager")
	}
	return nil
}
