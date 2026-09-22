package evaluationinput

import (
	"encoding/json"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type ReportInputAuditIssue struct {
	Code    string
	Message string
}

func ReportInputSchema(data []byte) uint {
	var envelope struct {
		SchemaVersion uint `json:"schema_version"`
	}
	if len(data) == 0 || json.Unmarshal(data, &envelope) != nil {
		return 0
	}
	return envelope.SchemaVersion
}

func BuildFreezeOptionsFromSnapshot(input *InputSnapshot, modelRef ModelRef, decisionKind modelcatalog.DecisionKind) ReportInputFreezeOptions {
	opts := ReportInputFreezeOptions{ModelRef: modelRef, DecisionKind: decisionKind}
	if input == nil {
		return opts
	}
	if frozen, ok := InterpretationAssetsFromSnapshot(input); ok {
		copy := frozen
		opts.Assets = &copy
	}
	if def, ok := DefinitionV2FromSnapshot(input); ok {
		opts.FactorCatalog = FactorCatalogFromDefinition(def.Measure)
		family, _ := modelcatalog.AlgorithmFamilyFromDecisionKind(decisionKind)
		if family == modelcatalog.AlgorithmFamilyFactorClassification && input.Model != nil && len(def.ReportMap.Sections) > 0 {
			section := def.ReportMap.Sections[0]
			opts.TypologyRouting = &TypologyRoutingFreeze{
				DecisionKind: input.Model.DecisionKind, ReportKind: section.Kind, AdapterKey: section.AdapterKey,
				TemplateID: section.TemplateID, TemplateVersion: section.TemplateVersion,
			}
		}
	}
	if behavioral, ok := BehavioralRatingPayload(input); ok && behavioral.Snapshot != nil && behavioral.Snapshot.Norming != nil {
		if tables := behavioral.Snapshot.Norming.NormTablesOrNil(); tables != nil {
			opts.Norming = &NormingFreeze{NormTables: tables}
		}
	}
	if tp, ok := TypologyPayload(input); ok && tp != nil {
		src := tp.Source
		opts.TypologySource = &src
	}
	return opts
}

func AuditReportInput(data []byte, modelRef ModelRef) []ReportInputAuditIssue {
	if _, err := SnapshotFromReportInput(data, modelRef); err != nil {
		return []ReportInputAuditIssue{{Code: "report_input.invalid", Message: err.Error()}}
	}
	return nil
}
