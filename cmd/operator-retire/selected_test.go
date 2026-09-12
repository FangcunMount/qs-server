package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelectedExitInputRequiresExactIdentity(t *testing.T) {
	valid := `{"OrgID":1,"ActorID":9,"OperatorID":7,"ExpectedVersion":3,"RequestID":"selected-exit","Reason":"reviewed exit"}`
	for _, body := range []string{valid, valid + ` {}`, `{"Unexpected":true}`, `null`} {
		p := filepath.Join(t.TempDir(), "input.json")
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := readSelectedCommand(p)
		if (err == nil) != (body == valid) {
			t.Fatal("invalid command handling", err)
		}
	}
}
