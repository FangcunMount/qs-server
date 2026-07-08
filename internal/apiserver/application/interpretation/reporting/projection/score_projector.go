package projection

import (
	"context"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/registry"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

// ScoreProjector 投影分数 在之后 interpretation 用于 synchronous 计分 paths。
type ScoreProjector interface {
	ExecutionIdentity() evaluation.ExecutionIdentity
	// Key 是deprecated; 使用 Execution身份()。
	Key() evaluation.ExecutionIdentity
	Project(ctx context.Context, outcome evaloutcome.Outcome) error
}

// ScoreProjectorRegistry 解析score 投影器 按 评估器键 或 机制键。
type ScoreProjectorRegistry interface {
	Resolve(key evaluation.ExecutionIdentity) ScoreProjector
	ResolveByMechanism(key registry.MechanismReportBuilderKey) ScoreProjector
}

// MechanismKeyedScoreProjector 暴露机制 路由 元数据 用于 score 投影器。
type MechanismKeyedScoreProjector interface {
	ScoreProjector
	MechanismKey() registry.MechanismReportBuilderKey
}

// MultiMechanismKeyedScoreProjector registers 额外 decision-granularity 机制键。
type MultiMechanismKeyedScoreProjector interface {
	MechanismKeyedScoreProjector
	MechanismKeys() []registry.MechanismReportBuilderKey
}
