package rest

import (
	"os"
	"strings"
	"testing"
)

func TestReliableSubmissionRoutesDoNotRestoreSubmitStatus(t *testing.T) {
	source, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if strings.Contains(text, "submit-status") {
		t.Fatal("legacy /answersheets/submit-status route must not be registered")
	}
	if !strings.Contains(text, "assessment-readiness") {
		t.Fatal("assessment readiness route is not registered")
	}
}
