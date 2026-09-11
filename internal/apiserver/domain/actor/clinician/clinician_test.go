package clinician

import "testing"

func TestClinicianLifecycle(t *testing.T) {
	item := NewClinician(1, "Alice", "psych", "主任", TypeCounselor, "E001", true)

	if !item.IsActive() {
		t.Fatalf("expected clinician to start active")
	}
	item.UpdateProfile("Bob", "children", "副主任", TypeDoctor, "E002")
	if item.Name() != "Bob" || item.Department() != "children" || item.Title() != "副主任" {
		t.Fatalf("expected clinician profile to update")
	}
	if item.ClinicianType() != TypeDoctor || item.EmployeeCode() != "E002" {
		t.Fatalf("expected clinician type and employee code to update")
	}

	item.Deactivate()
	if item.IsActive() {
		t.Fatalf("expected clinician to be inactive after deactivate")
	}

	item.Activate()
	if !item.IsActive() {
		t.Fatalf("expected clinician to be active after activate")
	}

}
