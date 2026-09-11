// Package testeestore defines persistence requirements for current service-store ownership.
package testeestore

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/store"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

type History struct {
	EntryID     *uint64
	ClinicianID *uint64
	ID          uint64
	OrgID       int64
	TesteeID    uint64
	FromStoreID *uint64
	ToStoreID   uint64
	Kind        string
	ActorID     int64
	CreatedAt   time.Time
	Reason      string
	RequestID   string
	Version     uint32
}

// Repository writes require the caller's transaction. Lock order is store then testee.
// It deliberately has no operations on clinician relations, assessments or QR codes.
type Repository interface {
	LockStore(context.Context, int64, uint64) (*store.Store, error)
	LockTestee(context.Context, int64, uint64) (*testee.Testee, error)
	FindChange(context.Context, int64, uint64, string) (*History, error)
	SaveOwnership(context.Context, *History, uint32) error
	AppendHistory(context.Context, *History) error
	History(context.Context, int64, uint64, uint64, int) ([]History, error)
}

// IntakeRepository probes the current clinician store before acquiring locks.
// The caller must lock the store, then clinician and entry, and recheck the probe.
type IntakeRepository interface {
	Repository
	ReadClinicianStore(context.Context, int64, uint64) (*uint64, error)
}
