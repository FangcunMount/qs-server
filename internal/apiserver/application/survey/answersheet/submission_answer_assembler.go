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
	answerDTOs []AnswerDTO,
	questionMap map[string]questionnaire.Question,
) ([]answerBuildResult, []ruleengine.AnswerValidationTask, error) {
	l.Infow("开始验证答案", "answer_count", len(answerDTOs), "action", "validate", "resource", "answer")

	results := make([]answerBuildResult, 0, len(answerDTOs))
	tasks := make([]ruleengine.AnswerValidationTask, 0, len(answerDTOs))

	for i, answerDTO := range answerDTOs {
		question, exists := questionMap[answerDTO.QuestionCode]
		if !exists {
			l.Warnw("问题不存在于问卷中", "question_code", answerDTO.QuestionCode, "answer_index", i, "result", "failed")
			return nil, nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "%s", fmt.Sprintf("问题 %s 不存在于问卷中", answerDTO.QuestionCode))
		}

		answerValue, err := answersheet.CreateAnswerValueFromRaw(
			questionnaire.QuestionType(answerDTO.QuestionType),
			answerDTO.Value,
		)
		if err != nil {
			l.Warnw("创建答案值失败", "question_code", answerDTO.QuestionCode, "question_type", answerDTO.QuestionType, "error", err.Error(), "result", "failed")
			return nil, nil, errors.WrapC(err, errorCode.ErrAnswerSheetInvalid, "%s", fmt.Sprintf("创建答案值失败 [%s]", answerDTO.QuestionCode))
		}

		results = append(results, answerBuildResult{
			questionCode: answerDTO.QuestionCode,
			answerValue:  answerValue,
			questionType: questionnaire.QuestionType(answerDTO.QuestionType),
		})

		tasks = append(tasks, ruleengine.AnswerValidationTask{
			ID:    answerDTO.QuestionCode,
			Value: answersheet.NewAnswerValueAdapter(answerValue),
			Rules: validationRuleSpecsFromQuestion(question),
		})
	}

	return results, tasks, nil
}

func validationRuleSpecsFromQuestion(question questionnaire.Question) []ruleengine.ValidationRuleSpec {
	rules := question.GetValidationRules()
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
