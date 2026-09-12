package assessment

import (
	"fmt"
	"math"
	"time"
)

// ConductingContext is the immutable Survey start evidence copied at intake.
// Current service ownership remains an Actor concern and never rewrites it.
type ConductingContext struct {
	id               uint64
	startedAt        time.Time
	storeID          uint64
	ownershipVersion uint32
	version          uint32
}

func NewConductingContext(id uint64, at time.Time, storeID *uint64, ownershipVersion, version uint32) (ConductingContext, error) {
	if id == 0 || id > math.MaxInt64 || at.IsZero() || version != 1 {
		return ConductingContext{}, fmt.Errorf("invalid conducting context")
	}
	c := ConductingContext{id: id, startedAt: at.UTC(), ownershipVersion: ownershipVersion, version: version}
	if storeID != nil {
		if *storeID == 0 || *storeID > math.MaxInt64 {
			return ConductingContext{}, fmt.Errorf("invalid conducting store")
		}
		c.storeID = *storeID
	}
	return c, nil
}
func (c ConductingContext) ID() uint64           { return c.id }
func (c ConductingContext) StartedAt() time.Time { return c.startedAt }
func (c ConductingContext) StoreID() *uint64 {
	if c.storeID == 0 {
		return nil
	}
	id := c.storeID
	return &id
}
func (c ConductingContext) OwnershipVersion() uint32 { return c.ownershipVersion }
func (c ConductingContext) Version() uint32          { return c.version }
func (c ConductingContext) Equal(other ConductingContext) bool {
	return c.id == other.id && c.startedAt.Equal(other.startedAt) && c.storeID == other.storeID && c.ownershipVersion == other.ownershipVersion && c.version == other.version
}
func (a *Assessment) ConductingContext() ConductingContext { return a.conductingContext }
func WithConductingContext(c ConductingContext) AssessmentOption {
	return func(a *Assessment) { a.conductingContext = c }
}
