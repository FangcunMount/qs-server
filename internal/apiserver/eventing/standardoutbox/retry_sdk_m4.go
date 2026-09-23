//go:build reliable_messaging_m4

package standardoutbox

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/outbox"
	"github.com/FangcunMount/reliable-messaging/relay"
	"github.com/FangcunMount/reliable-messaging/transport"
)

// SDKRetryPolicy is compiled only with the M4 candidate SDK that exposes
// Claim.FailureCount. It is not production wiring until that SDK is released.
func SDKRetryPolicy() relay.RetryPolicy {
	return func(claim outbox.Claim, outcome transport.Outcome) relay.RetryDecision {
		retry, _ := DecideDeliveryFailure(
			retrygovernance.OutboxPolicy(), claim.FailureCount,
			claim.Message.Input().ID, outcome, time.Now(),
		)
		return retry
	}
}
