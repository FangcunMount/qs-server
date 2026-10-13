package plan

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// TaskOpeningOptions makes the reminder producer choice independent of whether
// an Outbox profile happens to be available. Queued consumers use their own lifecycle.
type TaskOpeningOptions struct {
	ReminderEnabled bool
	ReminderOutbox  eventing.ProfileBinding
}

func (o TaskOpeningOptions) validate() error {
	if o.ReminderEnabled && o.ReminderOutbox.Stager == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "enabled task reminder requires a MySQL outbox profile")
	}
	return nil
}
