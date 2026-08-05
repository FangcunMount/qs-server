package configured

import (
	"fmt"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	personalityconfigured "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/runtime/configured"
	evalinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/input"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

// Adapter implements ModelAdapter 通过 配置化人格评估器。
type Adapter struct {
	evaluator personalityconfigured.Evaluator
}

// NewRuntimeAdapterWithEvaluator 返回运行时适配器 bound 到 特定 evaluator。
func NewRuntimeAdapterWithEvaluator(evaluator personalityconfigured.Evaluator) Adapter {
	return Adapter{evaluator: evaluator}
}

func (a Adapter) Score(
	payload *modeltypology.Payload,
	def *modeldefinition.Definition,
	sheet *evalinput.AnswerSheet,
) (outcometypology.ScoringResult, error) {
	if payload == nil {
		return outcometypology.ScoringResult{}, fmt.Errorf("typology payload is required")
	}
	return a.evaluator.Score(payload, def, sheet)
}
