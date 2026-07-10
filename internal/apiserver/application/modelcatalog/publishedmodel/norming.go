package publishedmodel

import (
	"encoding/json"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func buildNorming(model *domain.AssessmentModel) (*port.AssessmentSnapshot, error) {
	if model.Definition.IsEmpty() {
		return nil, fmt.Errorf("behavioral_rating model definition is empty")
	}
	if model.DefinitionV2 == nil {
		return nil, fmt.Errorf("behavioral_rating definition_v2 is required")
	}
	encoded := append([]byte(nil), model.Definition.Data...)
	if !json.Valid(encoded) {
		return nil, fmt.Errorf("behavioral_rating model definition is not valid json")
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = domain.AlgorithmBehavioralRatingDefault
	}
	decisionKind, err := model.DecisionKindForDefinition()
	if err != nil {
		return nil, err
	}
	return recordFromModel(model, domain.KindBehavioralRating, domain.SubKindEmpty, algorithm, domain.PayloadFormatForBehavioralRating(algorithm), decisionKind, encoded), nil
}
