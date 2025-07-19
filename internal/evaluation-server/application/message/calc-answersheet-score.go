package message

import (
	"context"
	"encoding/json"

	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/domain/calculation"
	grpcclient "github.com/yshujie/questionnaire-scale/internal/evaluation-server/infrastructure/grpc"
	"github.com/yshujie/questionnaire-scale/internal/pkg/pubsub"
	"github.com/yshujie/questionnaire-scale/pkg/log"

	answersheetpb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/answersheet"
	questionnairepb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/questionnaire"
)

// HandlerCalcAnswersheetScore 计算答卷分数处理器
type HandlerCalcAnswersheetScore struct {
	questionnaireClient *grpcclient.QuestionnaireClient
	answersheetClient   *grpcclient.AnswerSheetClient
}

var (
	questionnaire *questionnairepb.Questionnaire
	answersheet   *answersheetpb.AnswerSheet
)

// Handle 计算答卷得分，并保存分数
func (h *HandlerCalcAnswersheetScore) Handle(ctx context.Context, data pubsub.AnswersheetSavedData) error {
	log.Debugf("in HandlerCalcAnswersheetScore: %s", data)

	// 先加载答卷
	if err := h.loadAnswersheet(ctx, data.AnswerSheetID); err != nil {
		return err
	}

	// 从答卷中获取问卷代码，然后加载问卷
	if err := h.loadQuestionnaire(ctx, answersheet.QuestionnaireCode, answersheet.QuestionnaireVersion); err != nil {
		return err
	}

	// 计算答卷中每一个答案的得分
	if err := h.calculateAnswerScores(answersheet); err != nil {
		log.Errorf("计算答案得分失败: %v", err)
		return err
	}

	// 计算答卷总分
	if err := h.calculateAnswerSheetTotalScore(answersheet); err != nil {
		log.Errorf("计算答卷总分失败: %v", err)
		return err
	}

	// 保存答卷得分
	if err := h.saveAnswerSheetScores(ctx, data.AnswerSheetID, answersheet); err != nil {
		log.Errorf("保存答卷得分失败: %v", err)
		return err
	}

	log.Infof("答卷得分计算完成，答卷ID: %d, 总分: %d", data.AnswerSheetID, answersheet.Score)
	return nil
}

// loadQuestionnaire 加载问卷
func (h *HandlerCalcAnswersheetScore) loadQuestionnaire(ctx context.Context, questionnaireCode string, questionnaireVersion string) error {
	loadedQuestionnaire, err := h.questionnaireClient.GetQuestionnaire(ctx, questionnaireCode)
	if err != nil {
		return err
	}

	questionnaire = loadedQuestionnaire
	log.Debugf("loaded questionnaire: %s", questionnaire.String())
	return nil
}

// loadAnswersheet 加载答卷
func (h *HandlerCalcAnswersheetScore) loadAnswersheet(ctx context.Context, answerSheetID uint64) error {
	loadedAnswersheet, err := h.answersheetClient.GetAnswerSheet(ctx, answerSheetID)
	if err != nil {
		return err
	}

	answersheet = loadedAnswersheet
	log.Debugf("loaded answersheet: %s", answersheet.String())
	return nil
}

// calculateAnswerScores 计算答案得分
func (h *HandlerCalcAnswersheetScore) calculateAnswerScores(answersheet *answersheetpb.AnswerSheet) error {
	// 遍历答卷中的每个答案
	for _, answer := range answersheet.Answers {
		// 是否可以计算得分
		if !h.canCalculateScore(answer.QuestionCode) {
			continue
		}

		// 获取计算公式
		FormulaType := h.getCalculationFormulaType(answer.QuestionCode)

		// 根据计算规则，创建计算器
		calculater, err := calculation.GetCalculater(calculation.CalculaterType(FormulaType))
		if err != nil {
			log.Errorf("获取计算器失败: %v", err)
			continue
		}

		// 获取计算操作数（根据问题的 option 和 答案选中值）
		operands := h.loadCalculationOperands(answer.QuestionCode, answer.Value)

		// 执行计算
		score, err := calculater.Calculate(operands)
		if err != nil {
			log.Errorf("计算答案得分失败: %v", err)
			continue
		}

		// 保存计算结果
		answer.Score = uint32(score)
	}

	return nil
}

// canCalculateScore 是否可以计算得分
// 判断 question 是否拥有 CalculationRule、CalculationRule.FormulaType 不为空
func (h *HandlerCalcAnswersheetScore) canCalculateScore(questionCode string) bool {
	question := findQuestionByCode(questionCode)
	if question == nil {
		log.Debugf("question not found: %s", questionCode)
		return false
	}

	if question.CalculationRule == nil {
		log.Debugf("question calculation rule not found: %s", question.Title)
		return false
	}

	if question.CalculationRule.FormulaType == "" {
		log.Debugf("question calculation rule formula type is empty: %s", question.Title)
		return false
	}

	return true
}

// getCalculationFormulaType 获取计算公式类型
func (h *HandlerCalcAnswersheetScore) getCalculationFormulaType(questionCode string) string {
	// 获取问题
	question := findQuestionByCode(questionCode)
	if question == nil {
		log.Errorf("question not found: %s", questionCode)
		return ""
	}

	// 获取计算公式类型
	return question.CalculationRule.FormulaType
}

// loadCalculationOperands 获取计算操作数
func (h *HandlerCalcAnswersheetScore) loadCalculationOperands(questionCode string, answerValue string) (operands []calculation.Operand) {
	// 获取问题
	question := findQuestionByCode(questionCode)
	if question == nil {
		log.Errorf("question not found: %s", questionCode)
		return operands
	}

	// 解析答案值（可能是JSON字符串）
	var actualValue string
	if err := json.Unmarshal([]byte(answerValue), &actualValue); err != nil {
		// 如果不是JSON格式，直接使用原值
		actualValue = answerValue
	}

	log.Debugf("解析答案值: 原始值=%s, 解析后=%s", answerValue, actualValue)

	// 遍历问题选项
	for _, option := range question.Options {
		if option.Code == actualValue {
			operands = append(operands, calculation.Operand(option.Score))
			log.Debugf("找到匹配选项: %s, 得分: %d", option.Code, option.Score)
			break
		}
	}

	if len(operands) == 0 {
		log.Warnf("未找到匹配的选项: 问题=%s, 答案值=%s", questionCode, actualValue)
	}

	return
}

// calculateAnswerSheetTotalScore 计算答卷总分
func (h *HandlerCalcAnswersheetScore) calculateAnswerSheetTotalScore(answersheet *answersheetpb.AnswerSheet) error {
	var totalScore float64
	// 遍历答卷中的每个答案
	for _, answer := range answersheet.Answers {
		totalScore += float64(answer.Score)
	}

	answersheet.Score = uint32(totalScore)
	return nil
}

// saveAnswerSheetScores 保存答卷得分
func (h *HandlerCalcAnswersheetScore) saveAnswerSheetScores(ctx context.Context, answerSheetID uint64, answersheet *answersheetpb.AnswerSheet) error {
	log.Infof("保存答卷得分，答卷ID: %d, 总分: %d", answerSheetID, answersheet.Score)

	// 调用GRPC服务保存分数
	err := h.answersheetClient.SaveAnswerSheetScores(ctx, answerSheetID, answersheet.Score, answersheet.Answers)
	if err != nil {
		log.Errorf("保存答卷分数失败: %v", err)
		return err
	}

	log.Infof("答卷得分保存成功，答卷ID: %d, 总分: %d", answerSheetID, answersheet.Score)
	return nil
}

// findQuestionByCode 根据问题代码查找问题
func findQuestionByCode(questionCode string) *questionnairepb.Question {
	if questionnaire == nil {
		log.Errorf("questionnaire is nil, cannot find question: %s", questionCode)
		return nil
	}

	for _, question := range questionnaire.Questions {
		if question.Code == questionCode {
			return question
		}
	}
	return nil
}
