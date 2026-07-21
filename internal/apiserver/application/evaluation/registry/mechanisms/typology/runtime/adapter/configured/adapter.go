package configured

import (
	"fmt"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	personalityconfigured "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/runtime/configured"
	evalinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

// Adapter implements ModelAdapter 通过 配置化人格评估器。
type Adapter struct {
	algorithm modelcatalog.Algorithm
	evaluator personalityconfigured.Evaluator
}

// NewAdapter 返回配置化模型适配器 用于 given 算法别名。
func NewAdapter(algorithm modelcatalog.Algorithm) Adapter {
	return NewAdapterWithEvaluator(algorithm, personalityconfigured.NewEvaluator())
}

// NewAdapterWithEvaluator 返回配置化模型适配器 bound 到 特定 evaluator。
func NewAdapterWithEvaluator(algorithm modelcatalog.Algorithm, evaluator personalityconfigured.Evaluator) Adapter {
	return Adapter{
		algorithm: algorithm,
		evaluator: evaluator,
	}
}

func (a Adapter) Algorithm() modelcatalog.Algorithm {
	return a.algorithm
}

// NewRuntimeAdapter 返回配置化 adapter that 路由 purely 按 载荷运行时规格。
func NewRuntimeAdapter() Adapter {
	return NewRuntimeAdapterWithEvaluator(personalityconfigured.NewEvaluator())
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
	if a.algorithm != "" && payload.Algorithm != a.algorithm {
		return outcometypology.ScoringResult{}, fmt.Errorf(
			"typology algorithm %s does not match adapter %s",
			payload.Algorithm,
			a.algorithm,
		)
	}
	return a.evaluator.Score(payload, def, sheet)
}
