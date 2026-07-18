package answersheet

import (
	"os"
	"strings"
	"testing"
)

func TestReliableSubmissionArchitectureHasNoInProcessQueueOrSynchronousAssessment(t *testing.T) {
	source, err := os.ReadFile("submission_service.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, forbidden := range []string{"SubmitQueued", "SubmitQueue", ".EnsureAssessment("} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("reliable submission source contains forbidden legacy path %q", forbidden)
		}
	}
	if !strings.Contains(text, "AcceptDurably") {
		t.Fatal("reliable submission source must expose AcceptDurably")
	}
}
