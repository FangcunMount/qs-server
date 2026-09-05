package evaluation

import (
	"fmt"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	"strings"
)

const FailureTaxonomySchemaVersionV1 = "ai-explanation-failure-taxonomy/v1"

const (
	SemanticProviderNoMessage       = "semantic_provider_no_message"
	SemanticProviderRateLimited     = "semantic_provider_rate_limited"
	SemanticProviderFailed          = "semantic_provider_failed"
	SemanticResultUnknown           = "semantic_result_unknown"
	SemanticOutputMissingOrTooLarge = "semantic_output_missing_or_too_large"
	SemanticOutputSchemaInvalid     = "semantic_output_schema_invalid"
	SemanticOutputDecodeInvalid     = "semantic_output_decode_invalid"
	SemanticReceiptInvalid          = "semantic_receipt_invalid"
	SemanticDecisionContractInvalid = "semantic_decision_contract_invalid"
)

func IsSemanticExecutionFailureCode(code string) bool {
	switch strings.TrimSpace(code) {
	case SemanticProviderNoMessage, SemanticProviderRateLimited, SemanticProviderFailed,
		SemanticResultUnknown,
		SemanticOutputMissingOrTooLarge,
		SemanticOutputSchemaInvalid,
		SemanticOutputDecodeInvalid,
		SemanticReceiptInvalid,
		SemanticDecisionContractInvalid:
		return true
	default:
		return false
	}
}

type FailureStage string

const (
	FailureStageGenerationExecution     FailureStage = "generation_execution"
	FailureStageOutputValidation        FailureStage = "output_validation"
	FailureStageDeterministicValidation FailureStage = "deterministic_validation"
	FailureStageSemanticEvaluation      FailureStage = "semantic_evaluation"
	FailureStageHumanReview             FailureStage = "human_review"
	FailureStageRunGovernance           FailureStage = "run_governance"
)

func (s FailureStage) IsValid() bool {
	switch s {
	case FailureStageGenerationExecution,
		FailureStageOutputValidation,
		FailureStageDeterministicValidation,
		FailureStageSemanticEvaluation,
		FailureStageHumanReview,
		FailureStageRunGovernance:
		return true
	default:
		return false
	}
}

type FailureKind string

const (
	FailureKindInfrastructureExecution   FailureKind = "infrastructure_execution"
	FailureKindResultUnknown             FailureKind = "result_unknown"
	FailureKindProviderProtocol          FailureKind = "provider_protocol"
	FailureKindOutputContractConformance FailureKind = "output_contract_conformance"
	FailureKindSemanticExecution         FailureKind = "semantic_execution"
	FailureKindQualityFailure            FailureKind = "quality_failure"
)

func (k FailureKind) IsValid() bool {
	switch k {
	case FailureKindInfrastructureExecution,
		FailureKindResultUnknown,
		FailureKindProviderProtocol,
		FailureKindOutputContractConformance,
		FailureKindSemanticExecution,
		FailureKindQualityFailure:
		return true
	default:
		return false
	}
}

type FailureDisposition string

const (
	FailureDispositionRetryGeneration       FailureDisposition = "retry_generation"
	FailureDispositionReplaceGeneration     FailureDisposition = "replace_generation"
	FailureDispositionRetrySemantic         FailureDisposition = "retry_semantic"
	FailureDispositionManualAcknowledgement FailureDisposition = "manual_acknowledgement"
	FailureDispositionRetainCandidate       FailureDisposition = "retain_candidate"
	FailureDispositionRejectRelease         FailureDisposition = "reject_release"
	FailureDispositionCancelRun             FailureDisposition = "cancel_run"
	FailureDispositionNoAction              FailureDisposition = "no_action"
)

func (d FailureDisposition) IsValid() bool {
	switch d {
	case FailureDispositionRetryGeneration,
		FailureDispositionReplaceGeneration,
		FailureDispositionRetrySemantic,
		FailureDispositionManualAcknowledgement,
		FailureDispositionRetainCandidate,
		FailureDispositionRejectRelease,
		FailureDispositionCancelRun,
		FailureDispositionNoAction:
		return true
	default:
		return false
	}
}

// ClassifiedFailure is the domain representation of
// AIExplanationFailureTaxonomy v1. It never contains secret provider payloads;
// EvidenceRefs point to immutable checkpoints, receipts or outputs.
type ClassifiedFailure struct {
	ProviderDiagnostics *aiexplanation.ProviderFailureDiagnostics `json:"provider_diagnostics,omitempty" bson:"provider_diagnostics,omitempty"`
	SchemaVersion       string                                    `json:"schema_version"`
	Stage               FailureStage                              `json:"stage"`
	Kind                FailureKind                               `json:"kind"`
	Code                string                                    `json:"code"`
	Retryable           bool                                      `json:"retryable"`
	ResultUnknown       bool                                      `json:"result_unknown"`
	Disposition         FailureDisposition                        `json:"disposition"`
	SafeMessage         string                                    `json:"safe_message"`
	EvidenceRefs        []string                                  `json:"evidence_refs"`
}

func (f ClassifiedFailure) Validate() error {
	if f.ProviderDiagnostics != nil {
		if err := f.ProviderDiagnostics.Validate(); err != nil {
			return err
		}
	}
	if f.SchemaVersion != FailureTaxonomySchemaVersionV1 || !f.Stage.IsValid() || !f.Kind.IsValid() ||
		!f.Disposition.IsValid() || !policyIdentifierPattern.MatchString(f.Code) ||
		!validateEvidenceText(f.SafeMessage, 1000) || len(f.EvidenceRefs) < 1 || len(f.EvidenceRefs) > 16 {
		return fmt.Errorf("AI explanation classified failure is invalid")
	}
	seen := make(map[string]struct{}, len(f.EvidenceRefs))
	for _, ref := range f.EvidenceRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" || len(ref) > 256 || strings.ContainsAny(ref, "<>\t\r\n ") {
			return fmt.Errorf("AI explanation classified failure evidence reference is invalid")
		}
		if _, exists := seen[ref]; exists {
			return fmt.Errorf("AI explanation classified failure evidence reference is duplicated")
		}
		seen[ref] = struct{}{}
	}
	if f.ResultUnknown != (f.Kind == FailureKindResultUnknown) {
		return fmt.Errorf("AI explanation result-unknown classification is inconsistent")
	}
	switch f.Kind {
	case FailureKindResultUnknown:
		if f.Retryable || f.Disposition != FailureDispositionManualAcknowledgement {
			return fmt.Errorf("AI explanation result-unknown failure requires manual acknowledgement")
		}
	case FailureKindOutputContractConformance:
		if f.Stage != FailureStageOutputValidation || f.Retryable || f.Disposition != FailureDispositionReplaceGeneration {
			return fmt.Errorf("AI explanation output-contract failure disposition is invalid")
		}
	case FailureKindSemanticExecution:
		if f.Stage != FailureStageSemanticEvaluation || f.Disposition != FailureDispositionRetrySemantic {
			return fmt.Errorf("AI explanation semantic execution failure disposition is invalid")
		}
	case FailureKindQualityFailure:
		if f.Retryable || f.Disposition != FailureDispositionRetainCandidate && f.Disposition != FailureDispositionRejectRelease {
			return fmt.Errorf("AI explanation quality failure must retain the candidate or reject the release")
		}
	}
	return nil
}

func (f ClassifiedFailure) AllowsGenerationReplacement() bool {
	return f.Validate() == nil && f.Disposition == FailureDispositionReplaceGeneration
}

func (f ClassifiedFailure) AllowsSemanticRetry() bool {
	return f.Validate() == nil && f.Disposition == FailureDispositionRetrySemantic
}

func (f ClassifiedFailure) RequiresManualAcknowledgement() bool {
	return f.Validate() == nil && f.Disposition == FailureDispositionManualAcknowledgement
}

func (f ClassifiedFailure) CandidateExists() bool {
	if f.Validate() != nil {
		return false
	}
	return f.Kind == FailureKindSemanticExecution || f.Kind == FailureKindQualityFailure
}

func (f ClassifiedFailure) Clone() ClassifiedFailure {
	cloned := f
	if f.ProviderDiagnostics != nil {
		d := *f.ProviderDiagnostics
		cloned.ProviderDiagnostics = &d
	}
	cloned.EvidenceRefs = append([]string(nil), f.EvidenceRefs...)
	return cloned
}
