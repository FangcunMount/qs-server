// Package actorstore defines persistence capabilities for Actor store use cases.
package actorstore

import (
	"context"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/store"
	"time"
)

type Filter struct {
	Search         string
	Active         *bool
	Page, PageSize int
}
type Item struct {
	Store          *domain.Store
	ClinicianCount int64
}
type Page struct {
	Items []Item
	Total int64
}

// History is an immutable record of a clinician's store assignment.
type History struct {
	ID                uint64
	OrgID             int64
	ClinicianID       uint64
	FromStoreID       *uint64
	ToStoreID         uint64
	Kind              string
	ActorID           int64
	CreatedAt         time.Time
	Reason, RequestID string
	InvalidatedCount  int64
	Version           uint32
}

// Lock methods require the caller's transaction. Always lock store before clinician.
type Progress struct{ Total, Configured, Unconfigured, ActiveTotal, ActiveConfigured, ActiveUnconfigured int64 }

type Repository interface {
	Progress(context.Context, int64) (Progress, error)
	List(context.Context, int64, Filter) (Page, error)
	Get(context.Context, int64, uint64) (*domain.Store, error)
	LockStore(context.Context, int64, uint64) (*domain.Store, error)
	Create(context.Context, *domain.Store) error
	Save(context.Context, *domain.Store, uint32) error
	CountClinicians(context.Context, int64, uint64) (int64, error)
	LockClinician(context.Context, int64, uint64) (*clinician.Clinician, error)
	FindChange(context.Context, int64, uint64, string) (*History, error)
	SaveAssignment(context.Context, *History) error
	InvalidateEntries(context.Context, int64, uint64, int64, time.Time) (int64, error)
	AppendHistory(context.Context, *History) error
	History(context.Context, int64, uint64) ([]History, error)
}
