package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMinimumK6VersionPattern(t *testing.T) {
	for _, value := range []string{"k6 v1.5.0 (go1.25.1, darwin/arm64)", "k6 v1.6.2"} {
		if match := k6VersionPattern.FindStringSubmatch(value); len(match) != 4 {
			t.Fatalf("version %q did not match", value)
		}
	}
}

func TestVerdictPrecedence(t *testing.T) {
	if got := worseVerdict(VerdictIncomplete, VerdictFail); got != VerdictFail {
		t.Fatalf("got %s, want FAIL", got)
	}
	if got := worseVerdict(VerdictFail, VerdictError); got != VerdictError {
		t.Fatalf("got %s, want ERROR", got)
	}
}

func TestVerdictExitCodes(t *testing.T) {
	for status, want := range map[VerdictStatus]int{
		VerdictPass: 0, VerdictFail: 2, VerdictIncomplete: 3, VerdictError: 4,
	} {
		if got := exitCodeForVerdict(status); got != want {
			t.Fatalf("exitCodeForVerdict(%s) = %d, want %d", status, got, want)
		}
	}
}

func TestInvalidPlanStillWritesErrorArtifacts(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "runs")
	var stdout bytes.Buffer
	summary, exitCode, err := execute(context.Background(), runOptions{
		Plan: "unknown", Root: root, ConfigFile: filepath.Join(root, "missing.json"),
		OutputRoot: output, Stdout: &stdout, Stderr: &bytes.Buffer{},
	})
	if err == nil || exitCode != 4 || summary.Verdict.Status != VerdictError {
		t.Fatalf("result = verdict:%s exit:%d err:%v", summary.Verdict.Status, exitCode, err)
	}
	runDir := filepath.Join(output, summary.Run.ID)
	for _, name := range []string{"summary.json", "report.md", "raw-k6-summary.json", "evidence.json"} {
		if _, statErr := os.Stat(filepath.Join(runDir, name)); statErr != nil {
			t.Fatalf("%s was not written: %v", name, statErr)
		}
	}
}

func TestAdmissionPrerequisitesRequireEveryPriorPhaseToPass(t *testing.T) {
	expected := []phaseSpec{
		{ID: "smoke"},
		{ID: "experience_60"},
		{ID: "capacity_120"},
	}
	actual := []PhaseSummary{
		{ID: "smoke", Verdict: Verdict{Status: VerdictPass}},
		{ID: "experience_60", Verdict: Verdict{Status: VerdictIncomplete}},
		{ID: "capacity_120", Verdict: Verdict{Status: VerdictPass}},
	}

	verdict := admissionPrerequisiteVerdict(expected, actual)
	if verdict.Status != VerdictIncomplete {
		t.Fatalf("verdict = %#v, want INCOMPLETE", verdict)
	}
	if !strings.Contains(strings.Join(verdict.Reasons, "\n"), "experience_60") {
		t.Fatalf("reasons = %#v, want incomplete phase id", verdict.Reasons)
	}
}

func TestAdmissionPrerequisitesRejectMissingPhase(t *testing.T) {
	expected := []phaseSpec{{ID: "smoke"}, {ID: "capacity_280"}}
	actual := []PhaseSummary{{ID: "smoke", Verdict: Verdict{Status: VerdictPass}}}

	verdict := admissionPrerequisiteVerdict(expected, actual)
	if verdict.Status != VerdictIncomplete {
		t.Fatalf("verdict = %#v, want INCOMPLETE", verdict)
	}
}

func TestAdmissionWaitsForCleanBaselineBetweenEveryPhase(t *testing.T) {
	phases, err := phasesForPlan("admission")
	if err != nil {
		t.Fatal(err)
	}
	if recoveryID, previousID, required := interPhaseRecovery(phases, 0); required || recoveryID != "" || previousID != "" {
		t.Fatalf("first phase recovery = (%q, %q, %v), want none", recoveryID, previousID, required)
	}
	for index := 1; index < len(phases); index++ {
		recoveryID, previousID, required := interPhaseRecovery(phases, index)
		if !required {
			t.Fatalf("phase %s has no inter-phase recovery", phases[index].ID)
		}
		if want := "pre-" + phases[index].ID; recoveryID != want {
			t.Fatalf("phase %s recovery id = %q, want %q", phases[index].ID, recoveryID, want)
		}
		if previousID != phases[index-1].ID {
			t.Fatalf("phase %s recovers from %q, want %q", phases[index].ID, previousID, phases[index-1].ID)
		}
	}
}
