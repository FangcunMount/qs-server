package clinician

import "testing"

func TestClinicianLifecycleAndBinding(t *testing.T) {
	item := NewClinician(1, nil, "Alice", "psych", "主任", TypeCounselor, "E001", true)

	if !item.IsActive() {
		t.Fatalf("expected clinician to start active")
	}
	if item.OperatorID() != nil {
		t.Fatalf("expected clinician to start unbound")
	}

	item.BindOperator(42)
	if item.OperatorID() == nil || *item.OperatorID() != 42 {
		t.Fatalf("expected clinician to bind operator 42")
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

	item.UnbindOperator()
	if item.OperatorID() != nil {
		t.Fatalf("expected clinician to be unbound")
	}
}
