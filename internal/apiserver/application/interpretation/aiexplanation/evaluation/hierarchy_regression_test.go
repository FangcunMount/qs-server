package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	"os"
	"testing"
	"time"
)

// These synthetic outputs reproduce Run 635977982451659310: a valid first
// insight must not hide a parent/child violation in the second insight.
func TestV6KeepsRejectingObservedParentChildExtraInsight(t *testing.T) {
	suite, err := LoadV6()
	if err != nil {
		t.Fatal(err)
	}
	profile, err := buildProfile(suite.ProfileFixture, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var testCase Case
	for _, c := range suite.Cases {
		if c.CaseID == "PROMPT-EVAL-007" {
			testCase = c
		}
	}
	input, err := syntheticInput(testCase.ProviderPayload, profile, testCase.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	assertions := append(append([]Assertion(nil), suite.DefaultGenerationAssertions...), testCase.Expected.Assertions...)
	for _, slot := range []int{1, 2} {
		t.Run(fmt.Sprint(slot), func(t *testing.T) {
			raw, err := os.ReadFile(fmt.Sprintf("testdata/v5-hierarchy-slot-%d.json", slot))
			if err != nil {
				t.Fatal(err)
			}
			report, err := EvaluateCandidate(context.Background(), raw, input.Document, profile.Definition(), assertions, allowSafety{})
			if err != nil {
				t.Fatal(err)
			}
			failures := map[string]bool{}
			for _, a := range report.Assertions {
				if a.Status == AssertionFailed {
					failures[a.Type] = true
				}
			}
			if report.DeterministicHardGatePassed || !failures["profile_output_policy_satisfied"] || !failures["forbid_dimension_group"] {
				t.Fatalf("hierarchy violation lost: %#v", report)
			}
			var content output.Content
			if err := json.Unmarshal(raw, &content); err != nil {
				t.Fatal(err)
			}
			content.IntegratedInsights = content.IntegratedInsights[:1]
			report, err = EvaluateCandidate(context.Background(), marshalCandidate(t, content), input.Document, profile.Definition(), assertions, allowSafety{})
			if err != nil || !report.DeterministicHardGatePassed {
				t.Fatalf("sibling-only insight rejected: %#v / %v", report, err)
			}
		})
	}
}
