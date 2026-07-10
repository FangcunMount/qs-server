package publishedmodel

import (
	"encoding/json"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
)

func buildNorming(model *domain.AssessmentModel) (*port.AssessmentSnapshot, error) {
	if model.Definition.IsEmpty() {
		return nil, fmt.Errorf("behavioral_rating model definition is empty")
	}
	encoded := append([]byte(nil), model.Definition.Data...)
	if !json.Valid(encoded) {
		return nil, fmt.Errorf("behavioral_rating model definition is not valid json")
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = domain.AlgorithmBehavioralRatingDefault
	}
	var err error
	encoded, err = behavioral.PrepareDefinitionForPublish(encoded)
	if err != nil {
		return nil, err
	}
	return recordFromModel(model, domain.KindBehavioralRating, domain.SubKindEmpty, algorithm, domain.PayloadFormatForBehavioralRating(algorithm), behavioral.DecisionKindFromDefinitionPayload(encoded), encoded), nil
}
