package ruleset

import (
	_ "embed"
	"encoding/json"
	"fmt"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

//go:embed seed/mbti_oejts.json
var defaultMBTIModelJSON []byte

//go:embed seed/sbti_fun.json
var defaultSBTIModelJSON []byte

// LoadDefaultMBTILegacyModel loads the embedded MBTI seed model.
func LoadDefaultMBTILegacyModel() (*modeltypology.MBTILegacyModel, error) {
	var model modeltypology.MBTILegacyModel
	if err := json.Unmarshal(defaultMBTIModelJSON, &model); err != nil {
		return nil, fmt.Errorf("load default mbti model: %w", err)
	}
	if err := validateMBTILegacyModel(model); err != nil {
		return nil, err
	}
	return &model, nil
}

// LoadDefaultSBTILegacyModel loads the embedded SBTI seed model.
func LoadDefaultSBTILegacyModel() (*modeltypology.SBTILegacyModel, error) {
	var model modeltypology.SBTILegacyModel
	if err := json.Unmarshal(defaultSBTIModelJSON, &model); err != nil {
		return nil, fmt.Errorf("load default sbti model: %w", err)
	}
	if err := validateSBTILegacyModel(model); err != nil {
		return nil, err
	}
	return &model, nil
}

func validateMBTILegacyModel(model modeltypology.MBTILegacyModel) error {
	if model.Code == "" {
		return fmt.Errorf("mbti model code is required")
	}
	if model.Version == "" {
		return fmt.Errorf("mbti model version is required")
	}
	if model.QuestionnaireCode == "" {
		return fmt.Errorf("mbti questionnaire code is required")
	}
	if len(model.DimensionOrder) == 0 {
		return fmt.Errorf("mbti dimension order is required")
	}
	if len(model.QuestionMappings) == 0 {
		return fmt.Errorf("mbti question mappings are required")
	}
	if len(model.TypeProfiles) == 0 {
		return fmt.Errorf("mbti type profiles are required")
	}
	return nil
}

func validateSBTILegacyModel(model modeltypology.SBTILegacyModel) error {
	if model.Code == "" {
		return fmt.Errorf("sbti model code is required")
	}
	if model.Version == "" {
		return fmt.Errorf("sbti model version is required")
	}
	if model.QuestionnaireCode == "" {
		return fmt.Errorf("sbti questionnaire code is required")
	}
	if len(model.DimensionOrder) == 0 {
		return fmt.Errorf("sbti dimension order is required")
	}
	if len(model.QuestionMappings) == 0 {
		return fmt.Errorf("sbti question mappings are required")
	}
	if len(model.NormalOutcomes) == 0 {
		return fmt.Errorf("sbti normal outcomes are required")
	}
	return nil
}
