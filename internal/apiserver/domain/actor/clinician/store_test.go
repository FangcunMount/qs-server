package clinician

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/store"
	"testing"
	"time"
)

func TestAssignStoreOwnsCompanyAndVersionRules(t *testing.T) {
	cl := NewClinician(7, nil, "医生", "", "", Type("doctor"), "", false)
	cl.RestoreStore(nil, 1)
	target, err := store.New(2, 7, "A", "门店", "", 9, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	changed, err := cl.AssignStore(target, 1)
	if err != nil || !changed {
		t.Fatalf("inactive doctor first assignment: %v", err)
	}
	if cl.StoreID() == nil || *cl.StoreID() != 2 || cl.Version() != 2 {
		t.Fatal("assignment state not advanced")
	}
	changed, err = cl.AssignStore(target, 1)
	if err != nil || changed || cl.Version() != 2 {
		t.Fatal("same-store assignment is not idempotent")
	}
	foreign, err := store.New(3, 8, "B", "外部门店", "", 9, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cl.AssignStore(foreign, 2); err == nil {
		t.Fatal("cross-company assignment accepted")
	}
	if _, err = cl.AssignStore(nil, 2); err == nil {
		t.Fatal("store cleared")
	}
	if *cl.StoreID() != 2 || cl.Version() != 2 {
		t.Fatal("rejected assignment mutated clinician")
	}
}
