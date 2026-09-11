package testee

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/store"
)

func serviceStore(t *testing.T, id uint64, org int64) *store.Store {
	t.Helper()
	s, err := store.New(id, org, "TEST", "测试门店", "", 9, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestInitialStoreCannotSilentlyTransferTestee(t *testing.T) {
	subject := NewTestee(7, "受试者", Gender(0), nil)
	a, b := serviceStore(t, 1, 7), serviceStore(t, 2, 7)
	if subject.StoreID() != nil || subject.StoreVersion() != 1 {
		t.Fatal("new testee must be unassigned")
	}
	changed, err := subject.AssignInitialStore(a, 1)
	if err != nil || !changed {
		t.Fatalf("initial assignment: %v", err)
	}
	changed, err = subject.AssignInitialStore(a, 2)
	if err != nil || changed {
		t.Fatalf("same-store assignment: %v", err)
	}
	if _, err = subject.AssignInitialStore(b, 2); err == nil {
		t.Fatal("scan silently transferred an assigned testee")
	}
	if *subject.StoreID() != 1 || subject.StoreVersion() != 2 {
		t.Fatal("rejected assignment changed state")
	}
}

func TestTransferStoreRequiresExistingOwnershipAndCurrentVersion(t *testing.T) {
	subject := NewTestee(7, "受试者", Gender(0), nil)
	a, b := serviceStore(t, 1, 7), serviceStore(t, 2, 7)
	if _, err := subject.TransferStore(a, 1); err == nil {
		t.Fatal("transfer accepted unassigned testee")
	}
	if _, err := subject.AssignInitialStore(a, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := subject.TransferStore(b, 1); err == nil {
		t.Fatal("stale transfer accepted")
	}
	if *subject.StoreID() != 1 || subject.StoreVersion() != 2 {
		t.Fatal("conflict mutated ownership")
	}
	changed, err := subject.TransferStore(b, 2)
	if err != nil || !changed || *subject.StoreID() != 2 || subject.StoreVersion() != 3 {
		t.Fatalf("transfer: %v", err)
	}
	if subject.OrgID() != 7 || subject.Name() != "受试者" || subject.ProfileID() != nil {
		t.Fatal("transfer changed unrelated state")
	}
}

func TestStoreOwnershipRejectsInvalidTargetsWithoutMutation(t *testing.T) {
	inactive := serviceStore(t, 3, 7)
	if err := inactive.SetActive(false, 0, 1, 9, time.Now()); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]*store.Store{"missing": nil, "foreign": serviceStore(t, 2, 8), "inactive": inactive} {
		t.Run(name, func(t *testing.T) {
			subject := NewTestee(7, "受试者", Gender(0), nil)
			if _, err := subject.AssignInitialStore(target, 1); err == nil {
				t.Fatal("invalid initial target accepted")
			}
			if subject.StoreID() != nil || subject.StoreVersion() != 1 {
				t.Fatal("rejected initial assignment mutated state")
			}
			id := uint64(1)
			subject.RestoreStore(&id, 4)
			if _, err := subject.TransferStore(target, 4); err == nil {
				t.Fatal("invalid transfer target accepted")
			}
			if *subject.StoreID() != 1 || subject.StoreVersion() != 4 {
				t.Fatal("rejected transfer mutated state")
			}
		})
	}
}

func TestStoreOwnershipDoesNotExposeMutableReferences(t *testing.T) {
	subject := NewTestee(7, "受试者", Gender(0), nil)
	id := uint64(1)
	subject.RestoreStore(&id, 4)
	id = 2
	result := subject.StoreID()
	*result = 3
	if *subject.StoreID() != 1 || subject.StoreVersion() != 4 {
		t.Fatal("ownership mutated outside domain behavior")
	}
	subject.RestoreStore(nil, 1)
	if subject.StoreID() != nil {
		t.Fatal("unassigned persisted state not restored")
	}
}
