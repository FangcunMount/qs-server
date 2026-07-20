package evaluationinput

import (
	"encoding/json"
	"fmt"

	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ReportInputAuditIssue records a report-input integrity finding (MC-R017 batch 3).
type ReportInputAuditIssue struct {
	Code    string
	Message string
}

// UpgradeReportInputResult describes a report-input schema upgrade attempt.
type UpgradeReportInputResult struct {
	FromSchema uint
	ToSchema   uint
	Data       []byte
	Skipped    string
}

// ReportInputSchema detects the frozen report-input schema version.
func ReportInputSchema(data []byte) uint {
	if len(data) == 0 {
		return ReportInputSchemaLegacy
	}
	var peek struct {
		SchemaVersion *uint `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &peek); err != nil || peek.SchemaVersion == nil {
		return ReportInputSchemaLegacy
	}
	return *peek.SchemaVersion
}

// BuildFreezeOptionsFromSnapshot derives v3 freeze options from a decoded input snapshot.
func BuildFreezeOptionsFromSnapshot(input *InputSnapshot, modelRef ModelRef, family modelcatalog.AlgorithmFamily) ReportInputFreezeOptions {
	opts := ReportInputFreezeOptions{
		Payload:         input.ModelPayload,
		ModelRef:        modelRef,
		AlgorithmFamily: family,
	}
	if input == nil {
		return opts
	}
	if frozen, ok := InterpretationAssetsFromSnapshot(input); ok {
		copy := frozen
		opts.Assets = &copy
	}
	if def, ok := DefinitionV2FromSnapshot(input); ok {
		opts.FactorCatalog = FactorCatalogFromDefinition(def.Measure)
	} else if scale, ok := ScalePayload(input); ok {
		opts.FactorCatalog = FactorCatalogFromScale(scale)
	} else if behavioral, ok := BehavioralRatingPayload(input); ok {
		opts.FactorCatalog = FactorCatalogFromBehavioral(behavioral.Snapshot)
	}
	if behavioral, ok := BehavioralRatingPayload(input); ok {
		if behavioral.Snapshot != nil && behavioral.Snapshot.Norming != nil {
			if tables := behavioral.Snapshot.Norming.NormTablesOrNil(); tables != nil {
				opts.Norming = &NormingFreeze{NormTables: tables}
			}
		}
	}
	if tp, ok := TypologyPayload(input); ok && tp != nil {
		src := tp.Source
		opts.TypologySource = &src
	}
	return opts
}

// AuditReportInput verifies that frozen report input can be decoded for replay.
func AuditReportInput(data []byte, modelRef ModelRef) []ReportInputAuditIssue {
	if len(data) == 0 {
		return []ReportInputAuditIssue{{Code: "report_input.missing", Message: "report input is empty"}}
	}
	snapshot, err := SnapshotFromReportInput(data, modelRef)
	if err != nil {
		return []ReportInputAuditIssue{{Code: "report_input.decode_failed", Message: err.Error()}}
	}
	if _, ok := InterpretationAssetsFromSnapshot(snapshot); !ok {
		if schema := ReportInputSchema(data); schema < ReportInputSchemaV2 {
			return []ReportInputAuditIssue{{Code: "report_input.legacy_payload_only", Message: "legacy payload-only report input without interpretation assets"}}
		}
		return []ReportInputAuditIssue{{Code: "report_input.assets_missing", Message: "interpretation assets are not materialized"}}
	}
	return nil
}

// TryUpgradeReportInputToV3 upgrades legacy/v2 report input to schema v3 when replay-safe.
func TryUpgradeReportInputToV3(data []byte, modelRef ModelRef, family modelcatalog.AlgorithmFamily) (UpgradeReportInputResult, error) {
	return TryUpgradeReportInputToV3WithDefinition(data, modelRef, family, nil)
}

// TryUpgradeReportInputToV3WithDefinition upgrades legacy report input using optional published Definition (MC-R017 batch 5).
func TryUpgradeReportInputToV3WithDefinition(
	data []byte,
	modelRef ModelRef,
	family modelcatalog.AlgorithmFamily,
	def *modeldefinition.Definition,
) (UpgradeReportInputResult, error) {
	fromSchema := ReportInputSchema(data)
	if fromSchema >= ReportInputSchemaV3 {
		return UpgradeReportInputResult{
			FromSchema: fromSchema, ToSchema: fromSchema, Data: append([]byte(nil), data...), Skipped: "already_v3",
		}, nil
	}
	snapshot, err := SnapshotFromReportInput(data, modelRef)
	if err != nil {
		return UpgradeReportInputResult{}, fmt.Errorf("decode report input: %w", err)
	}
	if def != nil {
		AttachCanonicalDefinition(snapshot, def)
	}
	opts := BuildFreezeOptionsFromSnapshot(snapshot, modelRef, family)
	if !CanFreezeMinimalReportInput(opts) {
		return UpgradeReportInputResult{FromSchema: fromSchema, Skipped: "insufficient_minimal_snapshot"}, nil
	}
	upgraded, err := MarshalReportInput(opts)
	if err != nil {
		return UpgradeReportInputResult{}, fmt.Errorf("marshal report input v3: %w", err)
	}
	if _, err := SnapshotFromReportInput(upgraded, modelRef); err != nil {
		return UpgradeReportInputResult{}, fmt.Errorf("verify report input v3 replay: %w", err)
	}
	return UpgradeReportInputResult{
		FromSchema: fromSchema, ToSchema: ReportInputSchemaV3, Data: upgraded,
	}, nil
}
