package attentionprojection

import "context"

// Store persists attention projection state for retry and reconciliation.
type Store interface {
	EnsurePending(ctx context.Context, input PendingInput) (alreadySucceeded bool, err error)
	MarkSucceeded(ctx context.Context, eventID string) error
	RecordFailure(ctx context.Context, eventID string, errMsg string, maxAttempts int) (Status, error)
	GetByEventID(ctx context.Context, eventID string) (*Record, error)
	ListRetryable(ctx context.Context, maxAttempts int, limit int) ([]Record, error)
}
