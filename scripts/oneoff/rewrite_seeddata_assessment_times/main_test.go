package main

import "testing"

func TestUint64CSVFlagSetAcceptsRepeatedAndCommaSeparatedValues(t *testing.T) {
	var ids uint64CSVFlag
	if err := ids.Set("1001, 1002"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := ids.Set("1003"); err != nil {
		t.Fatalf("Set() second error = %v", err)
	}

	want := []uint64{1001, 1002, 1003}
	if len(ids) != len(want) {
		t.Fatalf("len(ids) = %d, want %d", len(ids), len(want))
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids[%d] = %d, want %d", i, ids[i], want[i])
		}
	}
}

func TestUint64CSVFlagSetRejectsInvalidValues(t *testing.T) {
	for _, raw := range []string{"0", "-1", "abc"} {
		var ids uint64CSVFlag
		if err := ids.Set(raw); err == nil {
			t.Fatalf("Set(%q) expected error", raw)
		}
	}
}

func TestValidateBackupSuffix(t *testing.T) {
	for _, suffix := range []string{"20260604_seeddata", "A_1"} {
		if err := validateBackupSuffix(suffix); err != nil {
			t.Fatalf("validateBackupSuffix(%q) error = %v", suffix, err)
		}
	}

	for _, suffix := range []string{"bad-suffix", "bad.suffix", ""} {
		if err := validateBackupSuffix(suffix); err == nil {
			t.Fatalf("validateBackupSuffix(%q) expected error", suffix)
		}
	}
}

func TestDateRawBeforeUsesYYYYMMDDLexicographicOrder(t *testing.T) {
	if !dateRawBefore("2026-05-30", "2026-06-04") {
		t.Fatal("expected earlier date to be before later date")
	}
	if dateRawBefore("2026-06-04", "2026-06-04") {
		t.Fatal("same date must not be before itself")
	}
}
