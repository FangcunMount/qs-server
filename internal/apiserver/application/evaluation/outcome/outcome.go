package outcome

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// Outcome 是规范 评估 写模型 passed 从 计分 到 interpretation。
type Outcome struct {
	Assessment           *assessment.Assessment
	Input                *evaluationinput.InputSnapshot
	Execution            *assessment.AssessmentOutcome
	RuntimeDescriptorKey evalpipeline.RuntimeDescriptorKey
}

// AssessmentID returns the stable assessment identity carried by either the
// live aggregate or a restored, persisted EvaluationOutcome.
func (o Outcome) AssessmentID() assessment.ID {
	if o.Assessment == nil {
		return 0
	}
	return o.Assessment.ID()
}

func (o Outcome) TesteeID() testee.ID {
	if o.Assessment == nil {
		return 0
	}
	return o.Assessment.TesteeID()
}
