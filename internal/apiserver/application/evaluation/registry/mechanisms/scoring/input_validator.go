package scoring

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/inputinvariant"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// ExecutionInput 是有效ated input 用于 因子计分 评估执行。
type ExecutionInput struct {
	Assessment *assessment.Assessment
	Input      *evaluationinput.InputSnapshot
	// DescriptorKey records the exact runtime descriptor invoking factor scoring.
	DescriptorKey string
}

// InputValidator 校验因子计分 execution input。
type InputValidator interface {
	Validate(input ExecutionInput) error
}

// 默认InputValidator 是production input 有效ator 用于 因子计分 runs。
type DefaultInputValidator struct{}

func (DefaultInputValidator) Validate(input ExecutionInput) error {
	if err := inputinvariant.Validate(inputinvariant.Input{
		Assessment:    input.Assessment,
		Snapshot:      input.Input,
		DescriptorKey: input.DescriptorKey,
	}); err != nil {
		return err
	}
	scale, ok := evaluationinput.ScalePayload(input.Input)
	if !ok || scale == nil {
		return fmt.Errorf("medical scale is required")
	}
	if len(scale.Factors) == 0 {
		return fmt.Errorf("medical scale has no factors")
	}
	if !scale.IsPublished() {
		return fmt.Errorf("medical scale is not published")
	}
	if modelRef := input.Assessment.EvaluationModelRef(); modelRef != nil && modelRef.IsScale() {
		if modelRef.Version() != "" && scale.ScaleVersion != "" && modelRef.Version() != scale.ScaleVersion {
			return fmt.Errorf("medical scale version does not match the evaluation model")
		}
	}
	return nil
}
