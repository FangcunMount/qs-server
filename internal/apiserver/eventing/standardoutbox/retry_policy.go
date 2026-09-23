package standardoutbox

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/relay"
	"github.com/FangcunMount/reliable-messaging/transport"
)

// DecideDeliveryFailure maps QS's publish-failure budget onto the SDK Relay.
// priorFailures is the count before this publish result; claim/reclaim counts
// must never be passed here. The returned disposition is for QS governance,
// while Relay receives only the scheduling/quarantine decision.
func DecideDeliveryFailure(policy retrygovernance.Policy, priorFailures uint64, eventID string, outcome transport.Outcome, at time.Time) (relay.RetryDecision, retrygovernance.Decision) {
	// The configured policy cannot exceed the hard cap. Saturating here keeps
	// malformed or very old counters exhausted without integer wraparound.
	attempt := retrygovernance.HardMaxOutboxAttempts + 1
	if priorFailures < retrygovernance.HardMaxOutboxAttempts {
		attempt = int(priorFailures) + 1
	}
	decision := policy.DecideFailureForKey(outcome == transport.Unknown, attempt, at, eventID)
	if decision.Disposition != retrygovernance.DispositionAutomatic || decision.NextAttemptAt == nil {
		return relay.RetryDecision{Quarantine: true}, decision
	}
	return relay.RetryDecision{Delay: decision.NextAttemptAt.Sub(at)}, decision
}
