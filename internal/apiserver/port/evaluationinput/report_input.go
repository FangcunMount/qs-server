package evaluationinput

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

const CurrentReportInputSchema uint = 3

type reportInputEnvelope struct {
	SchemaVersion        uint                         `json:"schema_version"`
	InterpretationAssets *interpretationassets.Assets `json:"InterpretationAssets"`
	ModelRef             *ModelRef                    `json:"model_ref"`
	FactorCatalog        []FactorCatalogEntry         `json:"factor_catalog,omitempty"`
	TypologySource       *typology.Source             `json:"typology_source,omitempty"`
	TypologyRouting      *TypologyRoutingFreeze       `json:"typology_routing,omitempty"`
	Norming              *NormingFreeze               `json:"norming,omitempty"`
}

// MarshalReportInput emits only the current minimal, payload-free schema.
func MarshalReportInput(opts ReportInputFreezeOptions) ([]byte, error) {
	if !CanFreezeMinimalReportInput(opts) {
		return nil, fmt.Errorf("report input schema %d freeze material is incomplete", CurrentReportInputSchema)
	}
	return json.Marshal(reportInputEnvelope{
		SchemaVersion: CurrentReportInputSchema, InterpretationAssets: opts.Assets,
		ModelRef: &opts.ModelRef, FactorCatalog: opts.FactorCatalog,
		TypologySource: opts.TypologySource, TypologyRouting: opts.TypologyRouting, Norming: opts.Norming,
	})
}

type decodedReportInput struct {
	InterpretationAssets *interpretationassets.Assets
	ModelRef             ModelRef
	FactorCatalog        []FactorCatalogEntry
	TypologySource       *typology.Source
	TypologyRouting      *TypologyRoutingFreeze
	Norming              *NormingFreeze
}

func decodeReportInputBytes(data []byte) (decodedReportInput, error) {
	if len(data) == 0 {
		return decodedReportInput{}, fmt.Errorf("report input is required")
	}
	var envelope reportInputEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return decodedReportInput{}, err
	}
	if envelope.SchemaVersion != CurrentReportInputSchema {
		return decodedReportInput{}, fmt.Errorf("unsupported report input schema %d", envelope.SchemaVersion)
	}
	if envelope.ModelRef == nil || envelope.ModelRef.Kind == "" || envelope.ModelRef.Code == "" {
		return decodedReportInput{}, fmt.Errorf("report input model_ref is required")
	}
	if envelope.InterpretationAssets == nil || !envelope.InterpretationAssets.IsMaterialized() {
		return decodedReportInput{}, fmt.Errorf("report input interpretation assets are required")
	}
	return decodedReportInput{
		InterpretationAssets: envelope.InterpretationAssets, ModelRef: *envelope.ModelRef,
		FactorCatalog: envelope.FactorCatalog, TypologySource: envelope.TypologySource,
		TypologyRouting: envelope.TypologyRouting, Norming: envelope.Norming,
	}, nil
}

// SnapshotFromReportInput accepts only schema 3.
func SnapshotFromReportInput(data []byte, model ModelRef) (*InputSnapshot, error) {
	decoded, err := decodeReportInputBytes(data)
	if err != nil {
		return nil, err
	}
	if model.Kind != "" && (model.Kind != decoded.ModelRef.Kind || model.Code != decoded.ModelRef.Code || model.Version != decoded.ModelRef.Version) {
		return nil, fmt.Errorf("report input model_ref does not match outcome model")
	}
	return snapshotFromMinimalReportInput(decoded.ModelRef, decoded)
}
