package aibridge

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
)

// MessagingAdmission borrows the host transaction and never commits or opens one.
// Closing takes an exclusive row lock; each submitter holds a shared row lock
// until its original submission transaction finishes. No process-local cache.
type MessagingAdmission struct{}

type MessagingAdmissionState struct {
	Closed    bool      `json:"closed"`
	Revision  uint64    `json:"revision"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (MessagingAdmission) Read(ctx context.Context, tx *sql.Tx) (MessagingAdmissionState, error) {
	return readMessagingAdmission(ctx, tx, " LOCK IN SHARE MODE")
}

// Inspect is a read-only snapshot, not an admission authorization.
func (MessagingAdmission) Inspect(ctx context.Context, tx *sql.Tx) (MessagingAdmissionState, error) {
	return readMessagingAdmission(ctx, tx, "")
}

func readMessagingAdmission(ctx context.Context, tx *sql.Tx, lock string) (MessagingAdmissionState, error) {
	var state MessagingAdmissionState
	if tx == nil {
		return state, app.ErrMessagingContract
	}
	err := tx.QueryRowContext(ctx, "SELECT closed,revision,updated_at FROM ai_messaging_admission WHERE singleton=1"+lock).Scan(&state.Closed, &state.Revision, &state.UpdatedAt)
	return state, err // including missing schema/row: unknown storage failure, not closure
}

// Set requires the reviewed revision, preventing a stale operator from opening
// admission after another operator has closed it. The host reports success only
// after committing this transaction.
func (MessagingAdmission) Set(ctx context.Context, tx *sql.Tx, closed bool, expectedRevision uint64) (MessagingAdmissionState, error) {
	state, err := readMessagingAdmission(ctx, tx, " FOR UPDATE")
	if err != nil {
		return state, err
	}
	if state.Revision != expectedRevision {
		return state, app.ErrConflict
	}
	if state.Closed == closed {
		return state, nil
	}
	if state.Revision == math.MaxUint64 {
		return state, errors.New("runtime admission revision exhausted")
	}
	_, err = tx.ExecContext(ctx, "UPDATE ai_messaging_admission SET closed=?,revision=revision+1,updated_at=UTC_TIMESTAMP(6) WHERE singleton=1", closed)
	if err != nil {
		return state, err
	}
	return readMessagingAdmission(ctx, tx, "")
}
