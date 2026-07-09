package taskperformance

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

type definitionPayload struct {
	factor.DefinitionBody
	SPM *spmExtension `json:"spm,omitempty"`
}

type spmExtension struct {
	TimeLimitSeconds int      `json:"time_limit_seconds,omitempty"`
	ItemSetCodes     []string `json:"item_set_codes,omitempty"`
	NormTableVersion string   `json:"norm_table_version,omitempty"`
}

// DefinitionFromPayload materializes the target cognitive definition model from
// the legacy draft payload without changing the payload contract.
func DefinitionFromPayload(payload []byte) (*definition.Definition, error) {
	var body definitionPayload
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, fmt.Errorf("decode cognitive definition: %w", err)
	}
	factors := factor.ParseLegacyFactorsFromDefinitionBody(body.Dimensions, body.InterpretRules)
	if body.SPM != nil {
		factors = ApplyNormMetadataToLegacyFactors(factors, MetadataContext{
			NormTableVersion: body.SPM.NormTableVersion,
			ItemSetCodes:     append([]string(nil), body.SPM.ItemSetCodes...),
		})
	}
	measure, calibration := definition.MeasureAndCalibrationFromLegacyFactors(factors)
	return &definition.Definition{
		Measure:     measure,
		Calibration: calibration,
	}, nil
}
