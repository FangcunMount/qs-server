package answersheet

import (
	"fmt"
	"math"
	"time"
)

type StartAttributionState string

const (
	StartCaptured   StartAttributionState = "captured"
	StartUnassigned StartAttributionState = "unassigned_at_start"
	StartLegacy     StartAttributionState = "legacy_start_not_captured"
)

// StartContext is an immutable business snapshot. Its zero value denotes a
// historical submission without a captured start, never the current store.
type StartContext struct {
	id               uint64
	startedAt        time.Time
	storeID          uint64
	ownershipVersion uint32
	version          uint32
}

func NewStartContext(id uint64, at time.Time, storeID *uint64, ownershipVersion uint32) (StartContext, error) {
	if id == 0 || id > math.MaxInt64 || at.IsZero() {
		return StartContext{}, fmt.Errorf("start identity and time are required")
	}
	c := StartContext{id: id, startedAt: at.UTC(), ownershipVersion: ownershipVersion, version: 1}
	if storeID != nil {
		if *storeID == 0 || *storeID > math.MaxInt64 {
			return StartContext{}, fmt.Errorf("invalid conducting store")
		}
		c.storeID = *storeID
	}
	return c, nil
}

func RestoreStartContext(id uint64, at time.Time, storeID *uint64, ownershipVersion, version uint32) (StartContext, error) {
	if version != 1 {
		return StartContext{}, fmt.Errorf("unsupported start context version")
	}
	return NewStartContext(id, at, storeID, ownershipVersion)
}
func (c StartContext) IsZero() bool             { return c.id == 0 }
func (c StartContext) ID() uint64               { return c.id }
func (c StartContext) StartedAt() time.Time     { return c.startedAt }
func (c StartContext) OwnershipVersion() uint32 { return c.ownershipVersion }
func (c StartContext) Version() uint32          { return c.version }
func (c StartContext) StoreID() *uint64 {
	if c.storeID == 0 {
		return nil
	}
	id := c.storeID
	return &id
}
func (c StartContext) State() StartAttributionState {
	if c.IsZero() {
		return StartLegacy
	}
	if c.storeID == 0 {
		return StartUnassigned
	}
	return StartCaptured
}
