package aibridge

import "context"

// CurrentMessagingOrganization is a read of current persisted responsibility
// and inherited delivery budgets. It is not broker/consumer or business-flow
// acceptance, and exposes no command identity or message body.
type CurrentMessagingOrganization struct {
	Requests        uint64
	CommandsPending uint64
	CommandAttempts uint64
}

// ReadCurrentMessagingOrganization borrows the host's connection/RO transaction
// and reuses the exact Runtime integrity and statistics expressions. It never
// acquires a pool connection, opens/ends a transaction, dispatches or settles.
func ReadCurrentMessagingOrganization(ctx context.Context, read RuntimeMessagingReadConnection, organizationID int64) (CurrentMessagingOrganization, error) {
	var out CurrentMessagingOrganization
	if ctx == nil || ctx.Err() != nil || read == nil || organizationID <= 0 {
		return out, ErrMessagingLedgerIntegrity
	}
	if err := requireRuntimeMessagingIntegrity(ctx, read, organizationID, ""); err != nil {
		return out, err
	}
	err := read.QueryRowContext(ctx, `SELECT COUNT(*),
 COALESCE(SUM(`+runtimePendingSelect+`),0),
 COALESCE(SUM(`+runtimeAttemptsSelect+`),0)
 FROM ai_bridge_requests r WHERE r.organization_id=?`, organizationID).
		Scan(&out.Requests, &out.CommandsPending, &out.CommandAttempts)
	if err != nil {
		return CurrentMessagingOrganization{}, err
	}
	if err = ctx.Err(); err != nil {
		return CurrentMessagingOrganization{}, err
	}
	return out, nil
}
