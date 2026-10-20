// Package tasktransaction describes reliable opening of one task schedule.
// It does not grant a new business retry or perform an external reminder.
package tasktransaction

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
)

// LocalGuard runs after the protocol guard in the original MySQL root. It
// rechecks ownership/admission using database operations only: no signing,
// clocks, event creation, network calls or independently committed writes.
type LocalGuard func(context.Context, *plan.AssessmentTask) error

type OpenRequest struct {
	Candidate *plan.AssessmentTask
	Guard     LocalGuard
}

// Opener freezes the first entry and reminder before preparing a half message.
// A repeated/unknown operation must be recovered without generating a new
// entry. The host owns the shared owner runtime and database lifecycle.
type Opener interface {
	RecoverOpened(context.Context, *plan.AssessmentTask) (*plan.AssessmentTask, bool, error)
	CommitOpened(context.Context, OpenRequest) (*plan.AssessmentTask, error)
}
