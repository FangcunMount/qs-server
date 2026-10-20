package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationconsistency"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

type outcomeWitnessChecker struct {
	decision transaction.Decision
	ref      transaction.Reference
	bounded  bool
}

func (c *outcomeWitnessChecker) Check(ctx context.Context, ref transaction.Reference) transaction.Decision {
	c.ref = ref
	deadline, ok := ctx.Deadline()
	c.bounded = ok && time.Until(deadline) <= 2*time.Second && time.Until(deadline) > 0
	return c.decision
}

func TestOutcomeTransactionAuditRequiresExactConfiguredAuthority(t *testing.T) {
	record := transactionOutcomeForCodec(t)
	result := &evaluationconsistency.CommittedOutboxEvidence{Status: string(transaction.Unknown)}
	reader := &consistencyReadModel{}
	reader.verifyCommittedTransaction(t.Context(), record, result)
	if result.InvalidReason == "" || result.Status != string(transaction.Unknown) {
		t.Fatal("unconfigured authority claimed commitment")
	}
	for _, decision := range []transaction.Decision{transaction.Commit, transaction.Unknown, transaction.Rollback} {
		checker := &outcomeWitnessChecker{decision: decision}
		reader.transactionChecker = checker
		result = &evaluationconsistency.CommittedOutboxEvidence{}
		reader.verifyCommittedTransaction(t.Context(), record, result)
		ref, err := transaction.DecodeReference(record.CommittedTransactionEvidence().TransactionReference)
		if err != nil {
			t.Fatal(err)
		}
		if checker.ref != ref || !checker.bounded {
			t.Fatal("audit did not query the exact reference with bounded fresh context")
		}
		if (result.InvalidReason == "") != (decision == transaction.Commit) || result.RowCount != 0 {
			t.Fatal("noncommit accepted or witness fabricated as Outbox row")
		}
	}
}
