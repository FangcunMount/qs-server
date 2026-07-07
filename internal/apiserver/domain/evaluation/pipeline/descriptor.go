package pipeline

import (
	"context"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// RuntimeDescriptorKey 路由评估执行 按 机制, 不 测评编码。
type RuntimeDescriptorKey struct {
	AlgorithmFamily modelcatalog.AlgorithmFamily
	DecisionKind    modelcatalog.DecisionKind
	PayloadFormat   string
}

func (k RuntimeDescriptorKey) IsZero() bool {
	return k.AlgorithmFamily == ""
}

func (k RuntimeDescriptorKey) String() string {
	parts := []string{k.AlgorithmFamily.String()}
	if k.DecisionKind != "" {
		parts = append(parts, string(k.DecisionKind))
	}
	if k.PayloadFormat != "" {
		parts = append(parts, k.PayloadFormat)
	}
	return strings.Join(parts, "/")
}

// CalculationInput 是机制无关 input passed 为 计算器。
type CalculationInput struct {
	Snapshot modelcatalog.PublishedModelSnapshot
}

// Calculator 运行计算 stage 用于 已发布模型快照。
type Calculator interface {
	Calculate(ctx context.Context, input CalculationInput) (any, error)
}

// InputAssembler 适配已发布快照 为 计算输入。
type InputAssembler interface {
	Assemble(snapshot modelcatalog.PublishedModelSnapshot) (CalculationInput, error)
}

// OutcomeAssembler 适配计算输出 为 规范 测评结果。
type OutcomeAssembler interface {
	Assemble(result any) (any, error)
}

// RuntimeDescriptor binds 机制身份 到 execution 协作者。
type RuntimeDescriptor struct {
	Key              RuntimeDescriptorKey
	AlgorithmFamily  modelcatalog.AlgorithmFamily
	PayloadFormat    string
	DecisionKind     modelcatalog.DecisionKind
	ExecutionPath    modelcatalog.ExecutionPath
	InputAssembler   InputAssembler
	Calculator       Calculator
	OutcomeAssembler OutcomeAssembler
}

// EvaluationPipeline 执行一个评估 用于 已发布模型快照。
type EvaluationPipeline interface {
	Supports(snapshot modelcatalog.PublishedModelSnapshot) bool
	Execute(ctx context.Context, snapshot modelcatalog.PublishedModelSnapshot) (any, error)
}
