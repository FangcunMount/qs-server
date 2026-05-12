package answersheet

import (
	"fmt"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// answerBuildResult 答案构建中间结果。
type answerBuildResult struct {
	questionCode string
	answerValue  answersheet.AnswerValue
	questionType questionnaire.QuestionType
}

// buildAnswerValuesAndTasks 构建答案值对象和校验任务。
func buildAnswerValuesAndTasks(
	l *logger.RequestLogger,
	spec questionnaire.SubmissionSpec,
	rawAnswers []questionnaire.RawSubmissionAnswer,
) ([]answerBuildResult, []ruleengine.AnswerValidationTask, error) {
	l.Infow("开始验证答案", "answer_count", len(rawAnswers), "action", "validate", "resource", "answer")

	preparedAnswers, err := spec.PrepareAnswers(rawAnswers)
	if err != nil {
		l.Warnw("提交答案不符合问卷规格", "error", err.Error(), "result", "failed")
		return nil, nil, errors.WrapC(err, errorCode.ErrAnswerSheetInvalid, "提交答案不符合问卷规格")
	}

	results := make([]answerBuildResult, 0, len(preparedAnswers))
	tasks := make([]ruleengine.AnswerValidationTask, 0, len(preparedAnswers))

	for _, prepared := range preparedAnswers {
		questionType := prepared.QuestionType()
		questionCode := prepared.QuestionCode().Value()
		answerValue, err := answersheet.CreateAnswerValueFromRaw(questionType, prepared.Value())
		if err != nil {
			l.Warnw("创建答案值失败", "question_code", questionCode, "question_type", questionType.Value(), "error", err.Error(), "result", "failed")
			return nil, nil, errors.WrapC(err, errorCode.ErrAnswerSheetInvalid, "%s", fmt.Sprintf("创建答案值失败 [%s]", questionCode))
		}

		results = append(results, answerBuildResult{
			questionCode: questionCode,
			answerValue:  answerValue,
			questionType: questionType,
		})

		tasks = append(tasks, ruleengine.AnswerValidationTask{
			ID:    questionCode,
			Value: answersheet.NewAnswerValueAdapter(answerValue),
			Rules: validationRuleSpecsFromPreparedAnswer(prepared),
		})
	}

	return results, tasks, nil
}

func rawSubmissionAnswersFromDTO(answerDTOs []AnswerDTO) []questionnaire.RawSubmissionAnswer {
	rawAnswers := make([]questionnaire.RawSubmissionAnswer, 0, len(answerDTOs))
	for _, answerDTO := range answerDTOs {
		rawAnswers = append(rawAnswers, questionnaire.RawSubmissionAnswer{
			QuestionCode: answerDTO.QuestionCode,
			QuestionType: answerDTO.QuestionType,
			Value:        answerDTO.Value,
		})
	}
	return rawAnswers
}

func validationRuleSpecsFromPreparedAnswer(answer questionnaire.PreparedSubmissionAnswer) []ruleengine.ValidationRuleSpec {
	rules := answer.ValidationRules()
	if len(rules) == 0 {
		return nil
	}
	specs := make([]ruleengine.ValidationRuleSpec, 0, len(rules))
	for _, rule := range rules {
		specs = append(specs, ruleengine.ValidationRuleSpec{
			RuleType:    ruleengine.ValidationRuleType(rule.GetRuleType()),
			TargetValue: rule.GetTargetValue(),
		})
	}
	return specs
}

// createAnswers 创建答案对象列表。
func createAnswers(l *logger.RequestLogger, results []answerBuildResult) ([]answersheet.Answer, error) {
	answers := make([]answersheet.Answer, 0, len(results))

	for _, ar := range results {
		answer, err := answersheet.NewAnswer(
			meta.NewCode(ar.questionCode),
			ar.questionType,
			ar.answerValue,
			0, // 初始分数为0，后续由评分系统计算
		)
		if err != nil {
			l.Errorw("创建答案对象失败", "question_code", ar.questionCode, "error", err.Error(), "result", "failed")
			return nil, errors.WrapC(err, errorCode.ErrAnswerSheetInvalid, "%s", fmt.Sprintf("创建答案失败 [%s]", ar.questionCode))
		}
		answers = append(answers, answer)
	}

	return answers, nil
}
