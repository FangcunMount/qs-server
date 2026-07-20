package evaluationinput

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	cognitivepayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

// RuntimeProjectionSource records whether runtime consumed canonical Definition or compat payload.
type RuntimeProjectionSource string

const (
	RuntimeProjectionCanonical RuntimeProjectionSource = "canonical_definition"
	RuntimeProjectionCompat    RuntimeProjectionSource = "compat_payload"
)

// TypologyRuntimeSpec prefers canonical Definition over compat payload (MC-R017 batch 5).
func TypologyRuntimeSpec(input *InputSnapshot, payload *modeltypology.Payload) (*modeltypology.RuntimeSpec, RuntimeProjectionSource, error) {
	if def, ok := DefinitionV2FromSnapshot(input); ok {
		spec, err := modeltypology.RuntimeSpecFromDefinition(def)
		if err != nil {
			return nil, "", err
		}
		return spec, RuntimeProjectionCanonical, nil
	}
	if payload == nil {
		return nil, "", nil
	}
	spec, err := payload.ToRuntimeSpec()
	if err != nil {
		return nil, "", err
	}
	return spec, RuntimeProjectionCompat, nil
}

// AbilityConclusionsFromSnapshot prefers canonical Definition conclusions (MC-R017 batch 5).
func AbilityConclusionsFromSnapshot(input *InputSnapshot) []conclusion.AbilityConclusion {
	if def, ok := DefinitionV2FromSnapshot(input); ok {
		return cognitivepayload.AbilityConclusions(def)
	}
	if cognitive, ok := CognitivePayload(input); ok && cognitive.Snapshot != nil {
		return cognitive.Snapshot.AbilityConclusions
	}
	return nil
}

// CognitiveExecutionSnapshot prefers canonical Definition projection over compat payload.
func CognitiveExecutionSnapshot(input *InputSnapshot) (*cognitivepayload.Snapshot, bool) {
	if def, ok := DefinitionV2FromSnapshot(input); ok {
		env := cognitiveDefinitionEnvelope(input)
		snapshot, err := cognitivepayload.SnapshotFromDefinition(env, def)
		if err == nil && snapshot != nil {
			return snapshot, true
		}
	}
	if cognitive, ok := CognitivePayload(input); ok && cognitive.Snapshot != nil {
		return cognitive.Snapshot, true
	}
	return nil, false
}

func cognitiveDefinitionEnvelope(input *InputSnapshot) cognitivepayload.DefinitionEnvelope {
	env := cognitivepayload.DefinitionEnvelope{Status: "published"}
	if input == nil {
		return env
	}
	if input.Model != nil {
		env.Code = input.Model.Code
		env.Version = input.Model.Version
		env.Title = input.Model.Title
	}
	if input.AnswerSheet != nil {
		env.QuestionnaireCode = input.AnswerSheet.QuestionnaireCode
		env.QuestionnaireVersion = input.AnswerSheet.QuestionnaireVersion
	}
	return env
}

// AuditRuntimeInputSource reports when runtime still depends on compat payload projection.
func AuditRuntimeInputSource(input *InputSnapshot) []ReportInputAuditIssue {
	if input == nil {
		return nil
	}
	issues := AuditLegacyIdentity(input)
	if _, ok := DefinitionV2FromSnapshot(input); ok {
		return issues
	}
	if input.ModelPayload != nil {
		issues = append(issues, ReportInputAuditIssue{
			Code:    "runtime.compat_payload_only",
			Message: "evaluation input lacks DefinitionV2; runtime falls back to compat payload projection",
		})
	}
	return issues
}

// AuditLegacyIdentity flags retained-read / empty algorithm identities (MC-R018).
func AuditLegacyIdentity(input *InputSnapshot) []ReportInputAuditIssue {
	if input == nil || input.Model == nil {
		return nil
	}
	kind := modelcatalog.Kind(input.Model.Kind)
	algorithm := modelcatalog.Algorithm(input.Model.Algorithm)
	raw := modelcatalog.AuditIdentityWritePolicy(kind, algorithm)
	if len(raw) == 0 {
		return nil
	}
	out := make([]ReportInputAuditIssue, 0, len(raw))
	for _, issue := range raw {
		out = append(out, ReportInputAuditIssue{Code: issue.Code, Message: issue.Message})
	}
	return out
}

// HasReportInputFreezeMaterial reports whether commit can freeze report input without legacy payload-only shape.
func HasReportInputFreezeMaterial(input *InputSnapshot) bool {
	if input == nil {
		return false
	}
	if input.ModelPayload != nil {
		return true
	}
	if def, ok := DefinitionV2FromSnapshot(input); ok {
		assets := def.ResolvedInterpretationAssets()
		return assets.IsMaterialized()
	}
	if input.InterpretationAssets != nil && input.InterpretationAssets.IsMaterialized() {
		return true
	}
	return false
}
