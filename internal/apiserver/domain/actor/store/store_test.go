package store

import (
	"testing"
	"time"
)

func TestStoreIdentityAndProfile(t *testing.T) {
	now := time.Now()
	s, err := New(1, 7, "  sh_a-1 ", "  上海门店 ", " 地址 ", 9, now)
	if err != nil {
		t.Fatal(err)
	}
	if s.Code() != "SH_A-1" || s.Name() != "上海门店" || !s.IsActive() {
		t.Fatalf("unexpected store: %+v", s.State())
	}
	if err = s.UpdateProfile("新名称", "新地址", 1, 10, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if s.Code() != "SH_A-1" || s.OrgID() != 7 || s.Version() != 2 || s.UpdatedBy() != 10 {
		t.Fatalf("immutable identity or audit changed: %+v", s.State())
	}
	before := s.State()
	if err = s.UpdateProfile("覆盖", "", 1, 11, now); err == nil {
		t.Fatal("stale version accepted")
	}
	if s.State() != before {
		t.Fatal("conflict mutated store")
	}
}

func TestStoreDeactivationRequiresNoCurrentClinicians(t *testing.T) {
	s, err := New(1, 7, "A", "门店", "", 9, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetActive(false, 1, 1, 9, time.Now()); err == nil {
		t.Fatal("assigned clinicians must prevent deactivation")
	}
	if !s.IsActive() || s.Version() != 1 {
		t.Fatal("failed deactivation changed state")
	}
	if err = s.SetActive(false, 0, 1, 9, time.Now()); err != nil {
		t.Fatal(err)
	}
	if s.IsActive() {
		t.Fatal("store not deactivated")
	}
	if err = s.SetActive(true, 0, 2, 9, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !s.IsActive() {
		t.Fatal("store not reactivated")
	}
}

func TestStoreRejectsInvalidCreation(t *testing.T) {
	for _, c := range []string{"", "a b", "a/b", "中文"} {
		t.Run(c, func(t *testing.T) {
			if _, err := New(1, 7, c, "name", "", 9, time.Now()); err == nil {
				t.Fatal("invalid code accepted")
			}
		})
	}
	if _, err := New(1, 0, "A", "name", "", 9, time.Now()); err == nil {
		t.Fatal("missing company accepted")
	}
	if _, err := New(1, 7, "A", " ", "", 9, time.Now()); err == nil {
		t.Fatal("empty name accepted")
	}
}
