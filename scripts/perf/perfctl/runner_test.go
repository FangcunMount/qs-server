package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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
