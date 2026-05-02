package pipeline

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// Context 评估上下文
// 在职责链中传递，携带评估所需的所有数据和中间结果
type Context struct {
	// 输入数据
	Assessment    *assessment.Assessment
	Input         *evaluationinput.InputSnapshot
	MedicalScale  *evaluationinput.ScaleSnapshot
	AnswerSheet   *evaluationinput.AnswerSheetSnapshot   // 答卷数据
	Questionnaire *evaluationinput.QuestionnaireSnapshot // 问卷数据（用于获取选项内容等）

	// 中间结果（由各处理器填充）
	FactorScores     []assessment.FactorScoreResult // 因子得分列表
	TotalScore       float64                        // 总分
	RiskLevel        assessment.RiskLevel           // 风险等级
	Conclusion       string                         // 总结论
	Suggestion       string                         // 总建议
	EvaluationResult *assessment.EvaluationResult   // 完整评估结果
	Report           *domainReport.InterpretReport  // 生成的报告（由 InterpretationHandler 填充）

	// 错误信息
	Error error
}

// NewContext 创建评估上下文
func NewContext(
	a *assessment.Assessment,
	input *evaluationinput.InputSnapshot,
) *Context {
	ctx := &Context{
		Assessment:   a,
		Input:        input,
		FactorScores: make([]assessment.FactorScoreResult, 0),
	}
	if input != nil {
		ctx.MedicalScale = input.MedicalScale
		ctx.AnswerSheet = input.AnswerSheet
		ctx.Questionnaire = input.Questionnaire
	}
	return ctx
}

// HasError 检查是否有错误
func (c *Context) HasError() bool {
	return c.Error != nil
}

// SetError 设置错误
func (c *Context) SetError(err error) {
	c.Error = err
}
