package evaluationinput

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	cognitivepayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
)

// AbilityConclusionsFromSnapshot prefers canonical Definition conclusions (MC-R017 batch 5).
func AbilityConclusionsFromSnapshot(input *InputSnapshot) []conclusion.AbilityConclusion {
	if def, ok := DefinitionV2FromSnapshot(input); ok {
		return cognitivepayload.AbilityConclusions(def)
	}
	return nil
}

// CognitiveExecutionSnapshot projects the canonical Definition into the calculation DTO.
func CognitiveExecutionSnapshot(input *InputSnapshot) (*cognitivepayload.Snapshot, bool) {
	if def, ok := DefinitionV2FromSnapshot(input); ok {
		env := cognitiveDefinitionEnvelope(input)
		snapshot, err := cognitivepayload.SnapshotFromDefinition(env, def)
		if err == nil && snapshot != nil {
			attachExactCognitiveNormTables(input, snapshot)
			return snapshot, true
		}
	}
	return nil, false
}

// attachExactCognitiveNormTables carries only immutable Norm material into the
// canonical DefinitionV2 projection. Executable semantics still come from the
// Definition; a missing or mismatched frozen Norm version remains fail-closed.
func attachExactCognitiveNormTables(input *InputSnapshot, canonical *cognitivepayload.Snapshot) {
	if canonical == nil || canonical.SPM == nil || !canonical.SPM.NormRequired {
		return
	}
	payload, ok := CognitivePayload(input)
	if !ok || payload.Snapshot == nil || payload.Snapshot.SPM == nil || payload.Snapshot.SPM.NormTables == nil {
		return
	}
	tables := payload.Snapshot.SPM.NormTables
	for _, item := range canonical.Factors {
		if item.Code != canonical.SPM.TotalFactorCode || item.Norm == nil {
			continue
		}
		if item.Norm.NormTableVersion == tables.NormTableVersion {
			canonical.SPM.NormTables = tables
		}
		return
	}
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

// AuditRuntimeInputSource requires canonical DefinitionV2 runtime material.
func AuditRuntimeInputSource(input *InputSnapshot) []ReportInputAuditIssue {
	if input == nil {
		return nil
	}
	issues := AuditInputIdentity(input)
	if _, ok := DefinitionV2FromSnapshot(input); ok {
		return issues
	}
	issues = append(issues, ReportInputAuditIssue{Code: "runtime.definition_v2_missing", Message: "evaluation input lacks DefinitionV2"})
	return issues
}

// AuditInputIdentity flags empty or unsupported runtime identities.
func AuditInputIdentity(input *InputSnapshot) []ReportInputAuditIssue {
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

// HasReportInputFreezeMaterial reports whether commit can freeze current report input.
func HasReportInputFreezeMaterial(input *InputSnapshot) bool {
	if input == nil {
		return false
	}
	if def, ok := DefinitionV2FromSnapshot(input); ok {
		assets := def.ResolvedInterpretationAssets()
		return assets.IsMaterialized()
	}
	return false
}
