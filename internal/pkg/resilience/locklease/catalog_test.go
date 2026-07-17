package locklease

import "testing"

func TestCatalogIsCompleteImmutableAndValid(t *testing.T) {
	if err := ValidateCatalog(); err != nil {
		t.Fatalf("ValidateCatalog() error = %v", err)
	}
	all := All()
	if len(all) != 8 {
		t.Fatalf("len(All()) = %d, want 8", len(all))
	}

	want := []WorkloadID{
		WorkloadAnswersheetProcessing,
		WorkloadPlanSchedulerLeader,
		WorkloadStatisticsSyncLeader,
		WorkloadStatisticsSync,
		WorkloadBehaviorPendingReconcile,
		WorkloadEvaluationConsistencyReconcile,
		WorkloadBehaviorJourneyScanLeader,
		WorkloadCollectionSubmit,
	}
	for index, id := range want {
		if all[index].ID != id {
			t.Fatalf("All()[%d].ID = %q, want %q", index, all[index].ID, id)
		}
		if capability, ok := Lookup(id); !ok || capability.ID != id {
			t.Fatalf("Lookup(%q) = %+v, %v", id, capability, ok)
		}
	}

	all[0].Spec.Name = "mutated"
	capability, _ := Lookup(WorkloadAnswersheetProcessing)
	if capability.Spec.Name == "mutated" {
		t.Fatal("All() exposed mutable catalog storage")
	}
}
