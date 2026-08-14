package main

import (
	"encoding/json"
	"testing"
)

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
		{"capacity_80", 80, "2m"},
		{"capacity_100", 100, "2m"},
		{"capacity_110", 110, "2m"},
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

func TestCeiling120PlanStopsAtCurrentHardwareBoundary(t *testing.T) {
	phases, err := phasesForPlan("ceiling-120")
	if err != nil {
		t.Fatal(err)
	}
	if len(phases) != 2 || phases[0].ID != "capacity_110" || phases[1].ID != "capacity_120" {
		t.Fatalf("ceiling-120 phases = %#v, want 110 then 120", phases)
	}
	if phases[1].TargetQPS != 120 {
		t.Fatalf("last target = %d, want 120", phases[1].TargetQPS)
	}
}

func TestProfileQPSAcceptsGeneratedIntegerRates(t *testing.T) {
	got := profileQPS(map[string]any{
		"qps": map[string]any{
			"medicalSubmit": 7,
			"medicalQuery":  29.0,
			"chainProbe":    json.Number("1"),
			"invalid":       "3",
		},
	})
	if got["medicalSubmit"] != 7 || got["medicalQuery"] != 29 || got["chainProbe"] != 1 {
		t.Fatalf("profile QPS = %#v, want generated integer and decoded numeric rates", got)
	}
	if _, exists := got["invalid"]; exists {
		t.Fatalf("profile QPS accepted a non-numeric value: %#v", got)
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
	for _, target := range []int{80, 100, 110, 120, 200, 240, 280, 300} {
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
