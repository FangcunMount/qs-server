package evaluation

import (
	"reflect"
	"testing"

	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
)

func TestFrozenSemanticObligationsPreservesFailedEvidenceAndRejectsInventoryDrift(t *testing.T) {
	assertions := []Assertion{{Type: "forbidden_claims_absent"}, {Type: "no_unprovided_fact"}}
	frozen := []domainevaluation.AssertionReceipt{
		{Type: assertions[0].Type, Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "deterministic-v1", Status: domainevaluation.AssertionFailed, Detail: "original failure"},
		{Type: assertions[1].Type, Scope: domainevaluation.AssertionScopeCase, Ordinal: 1, Hard: true, Evaluator: "semantic-required", Status: domainevaluation.AssertionPendingSemantic},
	}
	before := append([]domainevaluation.AssertionReceipt(nil), frozen...)
	obligations, err := frozenSemanticObligationsV2(frozen, assertions, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(obligations) != 2 || !reflect.DeepEqual(frozen, before) {
		t.Fatal("semantic obligations must retain both the failed and pending assertions without mutating evidence")
	}
	for _, tc := range []struct {
		name   string
		mutate func([]domainevaluation.AssertionReceipt) []domainevaluation.AssertionReceipt
	}{
		{"missing", func(r []domainevaluation.AssertionReceipt) []domainevaluation.AssertionReceipt { return r[:1] }},
		{"extra", func(r []domainevaluation.AssertionReceipt) []domainevaluation.AssertionReceipt {
			return append(r, r[0])
		}},
		{"order", func(r []domainevaluation.AssertionReceipt) []domainevaluation.AssertionReceipt {
			r[0], r[1] = r[1], r[0]
			return r
		}},
		{"scope", func(r []domainevaluation.AssertionReceipt) []domainevaluation.AssertionReceipt {
			r[0].Scope = domainevaluation.AssertionScopeCase
			return r
		}},
		{"ordinal", func(r []domainevaluation.AssertionReceipt) []domainevaluation.AssertionReceipt {
			r[0].Ordinal++
			return r
		}},
		{"hard gate", func(r []domainevaluation.AssertionReceipt) []domainevaluation.AssertionReceipt {
			r[0].Hard = false
			return r
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := tc.mutate(append([]domainevaluation.AssertionReceipt(nil), frozen...))
			if _, err := frozenSemanticObligationsV2(changed, assertions, 1); err == nil {
				t.Fatal("changed frozen inventory was accepted")
			}
		})
	}
}
