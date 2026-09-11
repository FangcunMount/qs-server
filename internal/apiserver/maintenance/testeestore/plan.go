// Package testeestore owns historical ownership migration; it does not evaluate authorization.
package testeestore

import (
	"fmt"
	"sort"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
)

type TesteeFact struct {
	ID      uint64
	OrgID   int64
	StoreID *uint64
	Version uint32
}
type ClinicianFact struct {
	ID              uint64
	OrgID           int64
	StoreID         *uint64
	Active, Deleted bool
}
type StoreFact struct {
	ID     uint64
	OrgID  int64
	Active bool
}
type RelationFact struct {
	ID, TesteeID, ClinicianID uint64
	OrgID                     int64
	Type                      relation.RelationType
	Active, Deleted           bool
	UnboundAt                 *time.Time
}
type Facts struct {
	Testees    []TesteeFact
	Clinicians []ClinicianFact
	Stores     []StoreFact
	Relations  []RelationFact
}
type Issue struct {
	Code                    string
	RelationID, ClinicianID uint64
}
type Candidate struct {
	TesteeID            uint64
	OrgID               int64
	CurrentStoreID      *uint64
	TargetStoreID       *uint64
	ExpectedVersion     uint32
	State               string
	EvidenceRelationIDs []uint64
	Issues              []Issue
}

// Plan produces one deterministic result per active testee. Existing ownership is never overwritten.
// Relations with ambiguous lifecycle data block a candidate rather than being silently discarded.
func Plan(f Facts) ([]Candidate, error) {
	clinicians := map[uint64]ClinicianFact{}
	stores := map[uint64]StoreFact{}
	testees := map[uint64]TesteeFact{}
	relations := map[uint64][]RelationFact{}
	seen := map[uint64]bool{}
	for _, v := range f.Testees {
		if v.ID == 0 || v.OrgID <= 0 {
			return nil, fmt.Errorf("invalid testee identity")
		}
		if _, ok := testees[v.ID]; ok {
			return nil, fmt.Errorf("duplicate testee %d", v.ID)
		}
		testees[v.ID] = v
	}
	for _, v := range f.Clinicians {
		if v.ID == 0 || v.OrgID <= 0 {
			return nil, fmt.Errorf("invalid clinician identity")
		}
		if _, ok := clinicians[v.ID]; ok {
			return nil, fmt.Errorf("duplicate clinician %d", v.ID)
		}
		clinicians[v.ID] = v
	}
	for _, v := range f.Stores {
		if v.ID == 0 || v.OrgID <= 0 {
			return nil, fmt.Errorf("invalid store identity")
		}
		if _, ok := stores[v.ID]; ok {
			return nil, fmt.Errorf("duplicate store %d", v.ID)
		}
		stores[v.ID] = v
	}
	for _, v := range f.Relations {
		if v.ID == 0 || seen[v.ID] {
			return nil, fmt.Errorf("invalid or duplicate relation %d", v.ID)
		}
		seen[v.ID] = true
		if _, ok := testees[v.TesteeID]; !ok {
			if v.Active && !v.Deleted {
				return nil, fmt.Errorf("active relation %d has no active testee", v.ID)
			}
			continue
		}
		relations[v.TesteeID] = append(relations[v.TesteeID], v)
	}
	results := make([]Candidate, 0, len(testees))
	for _, subject := range testees {
		c := Candidate{TesteeID: subject.ID, OrgID: subject.OrgID, CurrentStoreID: copyID(subject.StoreID), ExpectedVersion: subject.Version, State: "unresolved", EvidenceRelationIDs: []uint64{}, Issues: []Issue{}}
		if subject.StoreID != nil {
			current, ok := stores[*subject.StoreID]
			if !ok || current.OrgID != subject.OrgID {
				c.Issues = append(c.Issues, Issue{Code: "invalid_existing_store"})
			} else {
				c.State = "already_assigned"
			}
			results = append(results, c)
			continue
		}
		targets := map[uint64]bool{}
		for _, link := range relations[subject.ID] {
			if link.Deleted || !link.Active {
				continue
			}
			add := func(code string) {
				c.Issues = append(c.Issues, Issue{Code: code, RelationID: link.ID, ClinicianID: link.ClinicianID})
			}
			if link.OrgID != subject.OrgID {
				add("cross_company_relation")
				continue
			}
			if link.UnboundAt != nil {
				add("inconsistent_relation_lifecycle")
				continue
			}
			if link.Type == relation.RelationTypeCreator {
				continue
			}
			if !relation.GrantsAccess(link.Type) {
				add("unsupported_relation_type")
				continue
			}
			clinician, ok := clinicians[link.ClinicianID]
			if !ok || clinician.Deleted {
				add("missing_clinician")
				continue
			}
			if clinician.OrgID != subject.OrgID {
				add("cross_company_clinician")
				continue
			}
			if !clinician.Active {
				add("inactive_clinician")
				continue
			}
			if clinician.StoreID == nil {
				add("unconfigured_clinician")
				continue
			}
			target, ok := stores[*clinician.StoreID]
			if !ok {
				add("missing_store")
				continue
			}
			if target.OrgID != subject.OrgID {
				add("cross_company_store")
				continue
			}
			if !target.Active {
				add("inactive_store")
				continue
			}
			targets[target.ID] = true
			c.EvidenceRelationIDs = append(c.EvidenceRelationIDs, link.ID)
		}
		if len(targets) > 1 {
			c.Issues = append(c.Issues, Issue{Code: "multiple_stores"})
		}
		if len(c.Issues) == 0 && len(targets) == 1 {
			for id := range targets {
				v := id
				c.TargetStoreID = &v
			}
			c.State = "candidate"
		}
		if len(c.Issues) == 0 && len(targets) == 0 {
			// Approved policy: no management evidence remains unassigned.
			c.State = "deferred"
			c.Issues = append(c.Issues, Issue{Code: "no_management_relation"})
		}
		sort.Slice(c.EvidenceRelationIDs, func(i, j int) bool { return c.EvidenceRelationIDs[i] < c.EvidenceRelationIDs[j] })
		sort.Slice(c.Issues, func(i, j int) bool {
			a, b := c.Issues[i], c.Issues[j]
			if a.RelationID != b.RelationID {
				return a.RelationID < b.RelationID
			}
			return a.Code < b.Code
		})
		results = append(results, c)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].TesteeID < results[j].TesteeID })
	return results, nil
}
func copyID(id *uint64) *uint64 {
	if id == nil {
		return nil
	}
	v := *id
	return &v
}
