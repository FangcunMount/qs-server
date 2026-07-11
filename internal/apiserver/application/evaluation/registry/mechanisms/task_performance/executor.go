package task_performance

import (
	"context"
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	portevaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// Executor 运行task-performance 评估s via 共享 因子计分 engine。
type Executor struct {
	scoring *factorscoring.Executor
}

var _ evaluationexecute.Evaluator = (*Executor)(nil)

// NewExecutor 创建task-performance 评估 executor。
func NewExecutor(scorer ruleengine.ScaleFactorScorer) *Executor {
	return &Executor{scoring: factorscoring.NewExecutor(scorer)}
}

func (e *Executor) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityCognitiveDefault
}

func (e *Executor) Key() evaluation.ExecutionIdentity {
	return e.ExecutionIdentity()
}

func (e *Executor) ExecutionPath() modelcatalog.ExecutionPath {
	return modelcatalog.ExecutionPathCognitiveDescriptor
}

func (e *Executor) Execute(ctx context.Context, input evaluationexecute.ExecutionInput) (*domainoutcome.Execution, error) {
	if e == nil || e.scoring == nil {
		return nil, fmt.Errorf("task_performance evaluation executor is not configured")
	}
	cognitivePayload, ok := portevaluationinput.CognitivePayload(input.Input)
	if !ok || cognitivePayload.Snapshot == nil {
		return nil, fmt.Errorf("cognitive model payload is required")
	}
	scaleSnapshot := cognitivePayload.Snapshot.ToScaleSnapshot()
	outcome, err := e.scoring.Execute(ctx, evaluationexecute.ExecutionInput{
		Assessment: input.Assessment,
		Input:      factorscoring.CloneInputWithScaleSnapshot(input.Input, scaleSnapshot),
	})
	if err != nil {
		return nil, err
	}
	return ApplyAbilityConclusions(NormalizeOutcome(outcome), cognitivePayload.Snapshot.AbilityConclusions), nil
}

// ApplyAbilityConclusions projects optional DefinitionV2 ability ranges onto
// calculated cognitive factor results. No configured rule means no change.
func ApplyAbilityConclusions(outcome *domainoutcome.Execution, rules []conclusion.AbilityConclusion) *domainoutcome.Execution {
	if outcome == nil || len(rules) == 0 {
		return outcome
	}
	for i := range outcome.Dimensions {
		dimension := &outcome.Dimensions[i]
		if dimension.Score == nil {
			continue
		}
		for _, rule := range rules {
			if rule.ScoreBasis != conclusion.ScoreBasisRaw || rule.FactorCode != dimension.Code {
				continue
			}
			for _, item := range rule.Rules {
				if dimension.Score.Value < item.MinScore || dimension.Score.Value > item.MaxScore {
					continue
				}
				dimension.Level = &domainoutcome.ResultLevel{Code: item.Level, Label: item.Title}
				break
			}
		}
	}
	return outcome
}
