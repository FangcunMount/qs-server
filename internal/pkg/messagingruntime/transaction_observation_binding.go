package messagingruntime

import (
	"context"
	"errors"

	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/prometheus/client_golang/prometheus"
)

// Attach only the explicitly supplied primary store. Message properties cannot
// select databases. Constructing descriptors starts no reads or background work.
func attachProtocolMonitor(runtime *TransactionRuntime, store transaction.PreparedStore, limits transaction.Limits) error {
	reader, ok := store.(transaction.ObservationReader)
	if !ok {
		return errors.New("transaction primary store requires protocol observation")
	}
	if _, ok = store.(transaction.RecoveryReceiptRecorder); !ok {
		return errors.New("transaction primary store requires positive recovery audit")
	}
	read := func(ctx context.Context) (protocolSample, error) {
		snapshot, err := reader.ObserveProtocol(ctx)
		if err != nil {
			return protocolSample{}, err
		}
		if snapshot.AsOf.IsZero() {
			return protocolSample{}, ErrProtocolProtection
		}
		return protocolSample{reserved: snapshot.ReservedExecutions, preparing: snapshot.PreparingExecutions, gaps: snapshot.ConfirmationGapBranches,
			unaged: snapshot.UnagedConfirmationGapBranches, untrackedAborted: snapshot.UntrackedAbortedBranches,
			unobservedCommitted: snapshot.UnobservedCommittedBranches, conflicts: snapshot.MetadataConflicts,
			expiryAge: snapshot.PreparedExpiryAgeSeconds, confirmationAge: snapshot.OldestConfirmationObservationAgeSeconds}, nil
	}
	monitor, err := newProtocolMonitor(store.Binding().Owner, read, runtime.sender.Stats, runtime.admitted, limits, prometheus.DefaultRegisterer)
	if err != nil {
		return err
	}
	runtime.monitor = monitor
	runtime.cleanupBudget = limits.RPCTimeout
	return nil
}
