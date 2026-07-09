package publishedmodel

import (
	"encoding/json"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func buildTaskPerformance(model *domain.AssessmentModel) (*port.AssessmentSnapshot, error) {
	if model.Definition.IsEmpty() {
		return nil, fmt.Errorf("cognitive model definition is empty")
	}
	encoded := append([]byte(nil), model.Definition.Data...)
	if !json.Valid(encoded) {
		return nil, fmt.Errorf("cognitive model definition is not valid json")
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = domain.AlgorithmSPM
	}
	return recordFromModel(model, domain.KindCognitive, domain.SubKindEmpty, algorithm, domain.PayloadFormatForCognitive(algorithm), domain.DecisionKindAbilityLevel, encoded), nil
}
