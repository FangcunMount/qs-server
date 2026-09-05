package evaluation

import (
	"fmt"
	"strings"
	"time"
)

// Cancel stops future work without erasing evidence or refunding reserved calls.
// A prepared checkpoint has not dispatched; CAS arbitrates against dispatch.
func (e *PromptEvaluationEvidenceV2) Cancel(actor, reason string, discard bool, at time.Time) error {
	reason = strings.TrimSpace(reason)
	if e == nil || reason == "" || len(reason) > 1000 || e.Status.IsTerminal() || e.UnresolvedResultUnknownCount > 0 {
		return fmt.Errorf("%w: Run cannot be canceled; resolve unknown results first", ErrConflict)
	}
	if discard != (e.Status == EvidenceStatusAwaitingReview) {
		return fmt.Errorf("%w: discard is only for awaiting-review Runs", ErrConflict)
	}
	if e.execution != nil && e.execution.Phase != AttemptExecutionPrepared {
		return fmt.Errorf("%w: Provider execution is dispatching; wait for its result", ErrConflict)
	}
	cloned := e.Clone()
	var refs []string
	if cloned.execution != nil {
		refs = []string{cloned.execution.ID}
		cloned.execution = nil
	}
	cause := "operator_canceled"
	if discard {
		cause = "operator_discarded"
	}
	if err := cloned.Transition(EvidenceStatusCanceled, cause, actor, refs, at); err != nil {
		return err
	}
	cloned.StateTransitions[len(cloned.StateTransitions)-1].Reason = reason
	if err := cloned.Validate(); err != nil {
		return err
	}
	*e = cloned
	return nil
}
