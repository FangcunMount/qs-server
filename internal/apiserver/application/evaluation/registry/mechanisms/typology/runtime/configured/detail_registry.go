package configured

import (
	"fmt"

	calcclassification "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

// DetailInput 携带scored 状态 required 到 assemble typed 明细载荷。
type DetailInput struct {
	Payload   *modeltypology.Payload
	Spec      *modeltypology.RuntimeSpec
	Vector    calcclassification.ProfileVector
	Decision  calcclassification.DecisionSpec
	Candidate calcclassification.OutcomeCandidate
	Selected  SelectedOutcome
	Adapter   modeltypology.DetailAdapterKey
}

// SelectedOutcome 是配置化-运行时 视图 of 选中 model 结果。
type SelectedOutcome struct {
	Code       string
	Similarity float64
	Trigger    string
	Dimensions []DimensionLevel
}

// DimensionLevel 是intermediate SBTI 维度分 供 明细组装。
type DimensionLevel struct {
	Code     string
	Name     string
	Model    string
	RawScore float64
	Level    string
}

type detailAssemblerFunc func(DetailInput) (any, error)

// DetailAssemblerRegistry 解析明细组装器 按 adapter 键。
type DetailAssemblerRegistry struct {
	assemblers map[modeltypology.DetailAdapterKey]detailAssemblerFunc
}

// 默认DetailAssemblerRegistry 返回内置 类型学明细组装器。
func DefaultDetailAssemblerRegistry() DetailAssemblerRegistry {
	return NewDetailAssemblerRegistry()
}

// NewDetailAssemblerRegistry 返回内置 类型学明细组装器。
func NewDetailAssemblerRegistry() DetailAssemblerRegistry {
	return DetailAssemblerRegistry{
		assemblers: map[modeltypology.DetailAdapterKey]detailAssemblerFunc{
			modeltypology.DetailAdapterPersonalityType: assemblePersonalityTypeDetail,
			modeltypology.DetailAdapterTraitProfile:    assembleTraitProfileDetail,
		},
	}
}

// Len 报告数量 明细组装器 是 已注册。
func (r DetailAssemblerRegistry) Len() int {
	return len(r.assemblers)
}

// Register 返回注册表副本 使用 额外 或 覆盖 明细组装器。
func (r DetailAssemblerRegistry) Register(key modeltypology.DetailAdapterKey, assembler detailAssemblerFunc) DetailAssemblerRegistry {
	next := DetailAssemblerRegistry{assemblers: make(map[modeltypology.DetailAdapterKey]detailAssemblerFunc, len(r.assemblers)+1)}
	for k, v := range r.assemblers {
		next.assemblers[k] = v
	}
	next.assemblers[key] = assembler
	return next
}

func (r DetailAssemblerRegistry) Assemble(input DetailInput) (any, error) {
	if input.Adapter == "" {
		return nil, fmt.Errorf("detail adapter key is required")
	}
	assembler, ok := r.assemblers[input.Adapter]
	if !ok {
		return nil, fmt.Errorf("unsupported detail adapter key: %s", input.Adapter)
	}
	return assembler(input)
}
