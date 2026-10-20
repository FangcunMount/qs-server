package evaluationtransaction

import (
	"context"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"time"
)

// FailureRequest is already planned terminal state, not authorization to run
// an evaluator again. FutureRetry retains the original business schedule.
type FailureRequest struct {
	Assessment  *assessment.Assessment
	Run         evalrun.EvaluationRun
	Immediate   []event.DomainEvent
	FutureRetry event.DomainEvent
	RetryAt     time.Time
}

type FailureCommitter interface {
	CommitFailure(context.Context, FailureRequest) (*assessment.Assessment, *evalrun.EvaluationRun, error)
	RecoverPending(context.Context, *assessment.Assessment) (bool, error)
}
