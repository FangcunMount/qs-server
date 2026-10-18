package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestOperatorFamilyKeepsOperationAndConfigurationArguments(t *testing.T) {
	for _, tc := range []struct {
		args           []string
		family, action string
	}{
		{[]string{"prepare", "apply"}, "prepare", "apply"},
		{[]string{"recover", "status"}, "recover", "status"},
		{[]string{"retire", "selected", "verify"}, "retire", "selected-verify"},
		{[]string{"retire", "orphan", "apply"}, "retire", "orphan-apply"},
	} {
		flags := []string{"--config=/private/reviewed.yaml", "--iam.enabled=true"}
		family, action, remaining, err := parseOperatorArgs(append(tc.args, flags...))
		if err != nil || family != tc.family || action != tc.action || !reflect.DeepEqual(remaining, flags) {
			t.Fatalf("operation/config routing changed: %s %s %v %v", family, action, remaining, err)
		}
	}
}

func TestOperatorFamilyRejectsAmbiguousAndRetiredActionsBeforeConfiguration(t *testing.T) {
	for _, args := range [][]string{nil, {"apply"}, {"prepare", "verify"}, {"recover", "delete"}, {"retire", "selected"}, {"retire", "selected", "status"}, {"retire", "orphan", "delete"}} {
		if _, _, _, err := parseOperatorArgs(args); err == nil {
			t.Fatalf("ambiguous command accepted: %v", args)
		}
	}
	for _, action := range []string{"preflight", "apply", "status", "verify"} {
		_, _, _, err := parseOperatorArgs([]string{"retire", action, "--config=/must-not-load.yaml"})
		if err == nil || !strings.Contains(err.Error(), "historical batch retirement actions are retired") {
			t.Fatal("historical batch became reachable", err)
		}
	}
}
