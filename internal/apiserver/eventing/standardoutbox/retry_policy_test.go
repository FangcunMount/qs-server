package standardoutbox

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/transport"
)

func TestDecideDeliveryFailureKeepsQSFailureBudget(t *testing.T) {
	policy := retrygovernance.DefaultOutboxPolicy
	at := time.Date(2026, 9, 23, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	for _, tc := range []struct {
		name          string
		priorFailures uint64
		outcome       transport.Outcome
		want          retrygovernance.Disposition
	}{
		{"first failure", 0, transport.Unknown, retrygovernance.DispositionAutomatic},
		{"last automatic failure", 28, transport.Unknown, retrygovernance.DispositionAutomatic},
		{"default budget exhausted", 29, transport.Unknown, retrygovernance.DispositionManualRequired},
		{"manual attempt does not reset budget", 30, transport.Unknown, retrygovernance.DispositionManualRequired},
		{"oversized historical counter stays exhausted", ^uint64(0), transport.Unknown, retrygovernance.DispositionManualRequired},
		{"rejected is terminal", 0, transport.Rejected, retrygovernance.DispositionTerminal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			retry, decision := DecideDeliveryFailure(policy, tc.priorFailures, "event-1", tc.outcome, at)
			if decision.Disposition != tc.want {
				t.Fatalf("disposition=%s want=%s", decision.Disposition, tc.want)
			}
			if tc.want == retrygovernance.DispositionAutomatic {
				if retry.Quarantine || retry.Delay <= 0 || decision.NextAttemptAt == nil || retry.Delay != decision.NextAttemptAt.Sub(at) {
					t.Fatalf("automatic scheduling changed: %+v %+v", retry, decision)
				}
			} else if !retry.Quarantine || retry.Delay != 0 {
				t.Fatalf("nonautomatic message was rescheduled: %+v", retry)
			}
		})
	}
	short := policy
	short.MaxAutomaticAttempts = 2
	retry, decision := DecideDeliveryFailure(short, 1, "event-1", transport.Unknown, at)
	if !retry.Quarantine || decision.Disposition != retrygovernance.DispositionManualRequired {
		t.Fatalf("configured lower budget ignored: %+v %+v", retry, decision)
	}
}
