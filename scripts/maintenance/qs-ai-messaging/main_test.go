package main

import (
	"strings"
	"testing"
)

func TestMQMaintenanceCommandsRejectBeforeDatabaseSelection(t *testing.T) {
	t.Setenv("QS_AI_MESSAGING_AUDIT_DSN", "private-invalid-dsn")
	t.Setenv("QS_AI_MESSAGING_CONTROL_DSN", "private-invalid-dsn")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"audit", "-limit=0"}, "audit requires"},
		{[]string{"audit", "-action=open"}, "audit requires"},
		{[]string{"audit", "close"}, "audit requires"},
		{[]string{"admission"}, "explicit inspect"},
		{[]string{"admission", "close"}, "reviewed revision"},
		{[]string{"admission", "open", "-expected-revision=01"}, "reviewed revision"},
		{[]string{"admission", "inspect", "-expected-revision=0"}, "inspect does not change"},
		{[]string{"preflight", "--binding=/must-not-read.json"}, "unsupported MQ maintenance command"},
	} {
		err := run(tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "private-invalid-dsn") {
			t.Fatalf("invalid command reached resources or exposed material: %v: %v", tc.args, err)
		}
	}
}

func TestMQMaintenanceRedactsDatabaseBindingErrors(t *testing.T) {
	t.Setenv("QS_AI_MESSAGING_AUDIT_DSN", "")
	t.Setenv("QS_AI_MESSAGING_CONTROL_DSN", "private-control-input")
	if err := run([]string{"audit"}); err == nil || err.Error() != "read-only MQ database audit failed" {
		t.Fatal("audit borrowed the control binding or exposed an error", err)
	}
	t.Setenv("QS_AI_MESSAGING_AUDIT_DSN", "private-audit-input")
	t.Setenv("QS_AI_MESSAGING_CONTROL_DSN", "")
	if err := run([]string{"admission", "inspect"}); err == nil || err.Error() != "MQ runtime admission control failed; no closure or submission decision can be inferred" {
		t.Fatal("admission borrowed the audit binding or exposed an error", err)
	}
}

func TestMQMaintenanceArgumentBoundsAndReviewedRevision(t *testing.T) {
	for _, limit := range []string{"1", "1000"} {
		if _, err := auditArguments([]string{"-limit=" + limit}); err != nil {
			t.Fatal(err)
		}
	}
	for _, action := range []string{"close", "open"} {
		got, revision, err := admissionArguments([]string{action, "-expected-revision=0"})
		if err != nil || got != action || revision != "0" {
			t.Fatal("reviewed revision zero must remain valid", got, revision, err)
		}
	}
}
