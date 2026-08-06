package main

import "testing"

func TestAdmissionPlanStages(t *testing.T) {
	phases, err := phasesForPlan("admission")
	if err != nil {
		t.Fatal(err)
	}
	wants := []struct {
		id       string
		target   int
		duration string
	}{
		{"smoke", 4, "30s"},
		{"experience_60", 60, "5m"},
		{"capacity_120", 120, "2m"},
		{"capacity_200", 200, "3m"},
		{"capacity_240", 240, "4m"},
		{"capacity_280", 280, "3m"},
		{"admission_300", 300, "10m"},
	}
	if len(phases) != len(wants) {
		t.Fatalf("phase count = %d, want %d", len(phases), len(wants))
	}
	for index, want := range wants {
		got := phases[index]
		if got.ID != want.id || got.TargetQPS != want.target || got.Duration != want.duration {
			t.Fatalf("phase[%d] = %#v, want %#v", index, got, want)
		}
	}
}

func TestScaledWorkloadKeepsExactTargetAndChainProbe(t *testing.T) {
	canonical := map[string]float64{
		"medicalQuery": 80, "personalityQuery": 40,
		"questionnaireQuery": 13, "personalityQuestionnaireQuery": 13,
		"medicalSubmit": 19, "personalitySubmit": 5,
		"medicalWaitReport": 70, "behaviorWaitReport": 10,
		"personalityWaitReport": 20, "stats": 29, "chainProbe": 1,
	}
	for _, target := range []int{120, 200, 240, 280, 300} {
		got, err := scaledWorkload(canonical, target)
		if err != nil {
			t.Fatalf("target %d: %v", target, err)
		}
		total := 0
		for _, value := range got {
			total += value.(int)
		}
		if total != target {
			t.Fatalf("target %d allocated %d: %#v", target, total, got)
		}
		if got["chainProbe"] != 1 {
			t.Fatalf("target %d chainProbe = %v, want 1", target, got["chainProbe"])
		}
	}
}
