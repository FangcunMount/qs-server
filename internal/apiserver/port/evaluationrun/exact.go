package evaluationrun

import (
	"context"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
)

// ExactReader reads a historical attempt by its original identity, rather than
// substituting the latest attempt during message recovery.
type ExactReader interface {
	FindRunByID(context.Context, evalrun.ID) (*evalrun.EvaluationRun, error)
}
