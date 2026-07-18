package execution

import (
	"errors"
	"fmt"

	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

// FailedError reports a terminal InterpretationRun failure that has already
// been durably committed. Its retryability is therefore the only Worker
// retry decision for this attempt.
type FailedError struct {
	GenerationID domaingeneration.ID
	RunID        interpretationrun.ID
	Failure      interpretationrun.Failure
	Origin       retrygovernance.AttemptOrigin
	Decision     *retrygovernance.Decision
}

func (e *FailedError) Error() string {
	if e == nil {
		return "interpretation report generation failed"
	}
	return fmt.Sprintf("interpretation report generation failed: %s", e.Failure.Code)
}

func FailureFrom(err error) (*FailedError, bool) {
	var failed *FailedError
	if !errors.As(err, &failed) || failed == nil {
		return nil, false
	}
	return failed, true
}
