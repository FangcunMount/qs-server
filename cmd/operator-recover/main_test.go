package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveryInputRequiresExactIdentityAndOneObject(t *testing.T) {
	good := `{"OrgID":1,"ActorID":9,"OperatorID":7,"ExpectedVersion":3,"ExpectedPolicyVersion":10,"RequestID":"recovery-test","Reason":"restore reviewed headquarters identity"}`
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"valid", good, true}, {"extra object", good + ` {}`, false}, {"unknown field", `{"unexpected":true}`, false}, {"missing versions", `{"OrgID":1}`, false}, {"empty", "", false}, {"null", "null", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "input.json")
			if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := readCommand(path)
			if (err == nil) != tc.valid {
				t.Fatal("unexpected validation", err)
			}
		})
	}
}
