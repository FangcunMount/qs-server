package operatorretirement

import (
	"context"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operatorretirement"
)

// RecoveryRepository preserves immutable exit evidence and restores only local identity.
// Callers hold the same per-user exclusion as retirement and keep authorization writes paused.
type RecoveryRepository interface {
	Repository
	FindRecovery(context.Context, string) (*domain.Recovery, error)
	Recover(context.Context, domain.Recovery) error
}
