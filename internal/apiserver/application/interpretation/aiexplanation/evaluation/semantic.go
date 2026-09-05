package evaluation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
)

// SemanticAssertion is one unresolved obligation from the frozen suite. The
// one-based ordinal keeps repeated assertion types distinct.
type SemanticAssertion struct {
	Type       string
	Scope      domainevaluation.AssertionScope
	Ordinal    int
	Hard       bool
	Parameters Assertion
}

// SemanticEvaluationRequest contains only synthetic provider data and the
// normalized candidate output. Source report/user identifiers are deliberately
// excluded. Implementations must treat both JSON documents as untrusted data.
type SemanticEvaluationRequest struct {
	InvocationID string
	SuiteID      string
	CaseID       string
	Attempt      int
	InputJSON    []byte
	OutputJSON   []byte
	Assertions   []SemanticAssertion
}

type SemanticDecision struct {
	Type    string
	Scope   domainevaluation.AssertionScope
	Ordinal int
	Status  domainevaluation.AssertionStatus
	Detail  string
}

type SemanticEvaluationResult struct {
	EvaluatorVersion string
	Scores           domainevaluation.SemanticScores
	Rationale        string
	Decisions        []SemanticDecision
}

// SemanticEvaluationOutcome is returned after a semantic Provider dispatch.
// Controlled Provider/protocol/validation failures are data, not Go errors,
// so the runner can durably retain any raw output and receipt already obtained.
// Go errors are reserved for invalid internal requests or evaluator setup.
type SemanticEvaluationOutcome struct {
	InvocationID        string
	EvaluatorVersion    string
	StartedAt           time.Time
	FinishedAt          time.Time
	ProviderCallCount   int
	ProviderReceipt     *aiexplanation.ProviderReceipt
	RawOutput           []byte
	NormalizedOutput    []byte
	ProviderDiagnostics *aiexplanation.ProviderFailureDiagnostics
	ProviderFailureCode string
	Result              *SemanticEvaluationResult
	Failure             *domainevaluation.AttemptFailure
}

// SemanticEvaluator is intentionally separate from the generation Provider.
// An implementation may use an independent model plus rules, or a controlled
// human-assisted process, but must return one decision for every requested
// semantic assertion.
type SemanticEvaluator interface {
	Identity() domainevaluation.SemanticEvaluatorSpec
	Evaluate(ctx context.Context, request SemanticEvaluationRequest) (SemanticEvaluationOutcome, error)
}

func semanticReceipts(
	result SemanticEvaluationResult,
	providerReceipt *aiexplanation.ProviderReceipt,
	obligations []SemanticAssertion,
	expected domainevaluation.SemanticEvaluatorSpec,
	invocationID string,
) ([]domainevaluation.AssertionReceipt, *domainevaluation.SemanticReceipt, error) {
	if providerReceipt == nil {
		return nil, nil, fmt.Errorf("AI explanation semantic Provider receipt is required")
	}
	receipt := &domainevaluation.SemanticReceipt{
		EvaluatorVersion: strings.TrimSpace(result.EvaluatorVersion),
		ProviderReceipt:  *providerReceipt,
		Scores:           result.Scores,
		Rationale:        strings.TrimSpace(result.Rationale),
	}
	if err := receipt.Validate(); err != nil {
		return nil, nil, err
	}
	if receipt.EvaluatorVersion != expected.Version || receipt.ProviderReceipt.InvocationID != invocationID ||
		receipt.ProviderReceipt.Provider != expected.Provider.ResolvedProvider ||
		receipt.ProviderReceipt.Model != expected.Provider.ResolvedModel {
		return nil, nil, fmt.Errorf("AI explanation semantic evaluator receipt does not match the frozen evaluator")
	}
	want := make(map[string]SemanticAssertion, len(obligations))
	for _, obligation := range obligations {
		key := semanticAssertionKey(obligation.Type, obligation.Scope, obligation.Ordinal)
		if _, exists := want[key]; exists || strings.TrimSpace(obligation.Type) == "" || obligation.Ordinal < 1 {
			return nil, nil, fmt.Errorf("AI explanation semantic assertion inventory is invalid")
		}
		want[key] = obligation
	}
	if len(result.Decisions) != len(want) {
		return nil, nil, fmt.Errorf("AI explanation semantic evaluator returned %d decisions, want %d", len(result.Decisions), len(want))
	}
	seen := make(map[string]struct{}, len(result.Decisions))
	receipts := make([]domainevaluation.AssertionReceipt, 0, len(result.Decisions))
	for _, decision := range result.Decisions {
		key := semanticAssertionKey(decision.Type, decision.Scope, decision.Ordinal)
		obligation, exists := want[key]
		if !exists {
			return nil, nil, fmt.Errorf("AI explanation semantic evaluator returned an unknown assertion decision")
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, nil, fmt.Errorf("AI explanation semantic evaluator duplicated an assertion decision")
		}
		seen[key] = struct{}{}
		if decision.Status != domainevaluation.AssertionPassed && decision.Status != domainevaluation.AssertionFailed {
			return nil, nil, fmt.Errorf("AI explanation semantic assertion must resolve to passed or failed")
		}
		if strings.TrimSpace(decision.Detail) == "" {
			return nil, nil, fmt.Errorf("AI explanation semantic assertion rationale is required")
		}
		receipts = append(receipts, domainevaluation.AssertionReceipt{
			Type: decision.Type, Scope: decision.Scope, Ordinal: decision.Ordinal, Hard: obligation.Hard,
			Evaluator: receipt.EvaluatorVersion, Status: decision.Status, Detail: strings.TrimSpace(decision.Detail),
		})
	}
	return receipts, receipt, nil
}

func semanticAssertionKey(assertionType string, scope domainevaluation.AssertionScope, ordinal int) string {
	return string(scope) + "\x00" + assertionType + "\x00" + fmt.Sprint(ordinal)
}
