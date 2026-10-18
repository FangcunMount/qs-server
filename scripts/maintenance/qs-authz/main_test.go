package main

import (
	"reflect"
	"testing"
)

func TestAuthzFamilyRequiresExplicitReadOrProvisionSelection(t *testing.T) {
	for _, args := range [][]string{nil, {"--config=/must-not-load.yaml"}, {"apply"}, {"matrix"}} {
		if _, _, err := parseAuthzArgs(args); err == nil {
			t.Fatalf("implicit AuthZ action accepted: %v", args)
		}
	}
	for _, action := range []string{"verify", "provision"} {
		flags := []string{"--config=/private/reviewed.yaml"}
		got, remaining, err := parseAuthzArgs(append([]string{action}, flags...))
		if err != nil || got != action || !reflect.DeepEqual(remaining, flags) {
			t.Fatalf("action/config routing changed: %s %v %v", got, remaining, err)
		}
	}
}
