package retryschedule

import (
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

// DecisionDigest covers the durable authorization, excluding its storage-
// rounded timestamp. The exact due time is separately frozen in the plan and
// must be compared using the original backend's explicit persistence precision.
func DecisionDigest(d *retrygovernance.Decision) ([32]byte, error) {
	if d == nil || d.Disposition != retrygovernance.DispositionAutomatic || d.Attempt < 1 || d.RetryEventID == "" || d.PolicyVersion == "" || d.NextAttemptAt == nil || d.NextAttemptAt.IsZero() {
		return [32]byte{}, ErrInvalid
	}
	value := *d
	value.NextAttemptAt = nil
	raw, err := json.Marshal(value)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(raw), nil
}

func (f Frozen) MatchDecision(d *retrygovernance.Decision, persistedDue time.Time) error {
	hash, err := DecisionDigest(d)
	if err != nil || !f.Valid() || hash != f.Authority().DecisionHash || d.Attempt != f.Authority().ExpectedAttempt || d.RetryEventID != f.Message().Input().ID || d.ActionRequestID != f.Authority().RequestID || !d.NextAttemptAt.Equal(persistedDue) {
		return ErrConflict
	}
	return nil
}
