package operatorretirement

import (
	"context"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operatorretirement"
)

type Target struct {
	ID            uint64
	OrgID, UserID int64
	Version       uint32
}

// Repository operations must use the same per-user exclusion as registration,
// activation and role assignment. IAM writes are not part of a QS transaction.
type Repository interface {
	WithOperatorLock(context.Context, int64, uint64, func(context.Context) error) error
	Find(context.Context, int64, uint64) (Target, error)
	FindTask(context.Context, int64, uint64) (*domain.Task, error)
	OtherMemberships(context.Context, int64, uint64) (int64, error)
	// Begin atomically persists the task and disables the operator with a version check.
	Begin(context.Context, domain.Task) error
	Save(context.Context, domain.Task) error
	// Finish atomically saves completion and soft-deletes the disabled operator.
	Finish(context.Context, domain.Task) error
}

// MutationGate serializes all operator writes with retirement for the same IAM user.
type MutationGate interface {
	WithinMutation(context.Context, int64, func(context.Context) error) error
}
