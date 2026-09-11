package testeestore

import (
	"reflect"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
)

func ptr(v uint64) *uint64 { return &v }
func facts() Facts {
	return Facts{Testees: []TesteeFact{{ID: 1, OrgID: 7, Version: 1}}, Clinicians: []ClinicianFact{{ID: 10, OrgID: 7, StoreID: ptr(20), Active: true}, {ID: 11, OrgID: 7, StoreID: ptr(20), Active: true}}, Stores: []StoreFact{{ID: 20, OrgID: 7, Active: true}, {ID: 21, OrgID: 7, Active: true}}, Relations: []RelationFact{{ID: 30, OrgID: 7, TesteeID: 1, ClinicianID: 10, Type: relation.RelationTypeAttending, Active: true}, {ID: 31, OrgID: 7, TesteeID: 1, ClinicianID: 11, Type: relation.RelationTypeAssigned, Active: true}}}
}
func TestPlanCombinesSameStoreEvidenceDeterministically(t *testing.T) {
	f := facts()
	a, err := Plan(f)
	if err != nil {
		t.Fatal(err)
	}
	if a[0].State != "candidate" || *a[0].TargetStoreID != 20 || len(a[0].EvidenceRelationIDs) != 2 {
		t.Fatalf("candidate: %+v", a)
	}
	f.Relations[0], f.Relations[1] = f.Relations[1], f.Relations[0]
	b, err := Plan(f)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("plan depends on row order")
	}
}
func TestPlanStopsAmbiguousCandidates(t *testing.T) {
	cases := map[string]func(*Facts){
		"multiple_stores":                 func(f *Facts) { f.Clinicians[1].StoreID = ptr(21) },
		"unconfigured_clinician":          func(f *Facts) { f.Clinicians[1].StoreID = nil },
		"inactive_clinician":              func(f *Facts) { f.Clinicians[1].Active = false },
		"cross_company_store":             func(f *Facts) { f.Stores[0].OrgID = 8 },
		"inconsistent_relation_lifecycle": func(f *Facts) { now := time.Now(); f.Relations[0].UnboundAt = &now },
		"unsupported_relation_type":       func(f *Facts) { f.Relations[0].Type = "unknown" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := facts()
			mutate(&f)
			rows, err := Plan(f)
			if err != nil {
				t.Fatal(err)
			}
			if rows[0].State != "unresolved" || rows[0].TargetStoreID != nil {
				t.Fatal("ambiguous facts produced automatic assignment")
			}
			found := false
			for _, issue := range rows[0].Issues {
				if issue.Code == name {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing issue %s", name)
			}
		})
	}
}
func TestPlanNeverUsesCreatorOrInactiveRelations(t *testing.T) {
	f := facts()
	f.Relations[0].Type = relation.RelationTypeCreator
	f.Relations[1].Active = false
	rows, err := Plan(f)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].State != "deferred" || rows[0].Issues[0].Code != "no_management_relation" {
		t.Fatal("source-only relationship granted ownership")
	}
}
func TestPlanPreservesAlreadyAssignedStore(t *testing.T) {
	f := facts()
	f.Testees[0].StoreID = ptr(21)
	rows, err := Plan(f)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].State != "already_assigned" || rows[0].TargetStoreID != nil || *rows[0].CurrentStoreID != 21 {
		t.Fatal("existing ownership overwritten")
	}
	*rows[0].CurrentStoreID = 20
	if *f.Testees[0].StoreID != 21 {
		t.Fatal("output mutates input facts")
	}
}
func TestPlanRejectsDuplicateFactsAndOrphanedActiveRelations(t *testing.T) {
	f := facts()
	f.Clinicians = append(f.Clinicians, f.Clinicians[0])
	if _, err := Plan(f); err == nil {
		t.Fatal("duplicate facts accepted")
	}
	f = facts()
	f.Relations[0].TesteeID = 999
	if _, err := Plan(f); err == nil {
		t.Fatal("orphan silently discarded")
	}
}
