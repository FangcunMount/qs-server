package pipeline

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
)

// Context 评估上下文
// 在职责链中传递，携带评估所需的所有数据和中间结果
type Context struct {
	// 输入数据
	Assessment    *assessment.Assessment
	MedicalScale  *scale.MedicalScale
	AnswerSheet   *answersheet.AnswerSheet     // 答卷数据
	Questionnaire *questionnaire.Questionnaire // 问卷数据（用于获取选项内容等）

	// 中间结果（由各处理器填充）
	FactorScores     []assessment.FactorScoreResult // 因子得分列表
	TotalScore       float64                        // 总分
	RiskLevel        assessment.RiskLevel           // 风险等级
	Conclusion       string                         // 总结论
	Suggestion       string                         // 总建议
	EvaluationResult *assessment.EvaluationResult   // 完整评估结果
	Report           *domainReport.InterpretReport // 生成的报告（由 InterpretationHandler 填充）

	// 错误信息
	Error error
}

// NewContext 创建评估上下文
func NewContext(
	a *assessment.Assessment,
	medicalScale *scale.MedicalScale,
	answerSheet *answersheet.AnswerSheet,
) *Context {
	return &Context{
		Assessment:   a,
		MedicalScale: medicalScale,
		AnswerSheet:  answerSheet,
		FactorScores: make([]assessment.FactorScoreResult, 0),
	}
}

// HasError 检查是否有错误
func (c *Context) HasError() bool {
	return c.Error != nil
}

// SetError 设置错误
func (c *Context) SetError(err error) {
	c.Error = err
}
