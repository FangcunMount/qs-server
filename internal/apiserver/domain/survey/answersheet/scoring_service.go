package answersheet

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ==================== 答案值适配器 ====================

// answerValueAdapter 将 AnswerValue 适配为 calculation.ScorableValue
type answerValueAdapter struct {
	value AnswerValue
}

// NewScorableValue 创建可计分值适配器
func NewScorableValue(value AnswerValue) calculation.ScorableValue {
	return &answerValueAdapter{value: value}
}

func (a *answerValueAdapter) IsEmpty() bool {
	return a.value == nil || a.value.Raw() == nil
}

func (a *answerValueAdapter) AsSingleSelection() (string, bool) {
	if a.value == nil {
		return "", false
	}
	raw := a.value.Raw()
	if str, ok := raw.(string); ok {
		return str, true
	}
	return "", false
}

func (a *answerValueAdapter) AsMultipleSelections() ([]string, bool) {
	if a.value == nil {
		return nil, false
	}
	raw := a.value.Raw()

	switch v := raw.(type) {
	case []string:
		return v, true
	case []interface{}:
		// 处理从JSON反序列化的情况
		result := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result, len(result) > 0
	}
	return nil, false
}

func (a *answerValueAdapter) AsNumber() (float64, bool) {
	if a.value == nil {
		return 0, false
	}
	raw := a.value.Raw()

	switch v := raw.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	}
	return 0, false
}

// ==================== 计分服务接口 ====================

// ScoringService 答卷计分领域服务
// 职责：根据问卷的选项配置计算答案得分
// 设计原则：不关心题型，只关心选项和值的匹配
type ScoringService interface {
	// CalculateAnswerScore 计算单个答案的得分
	CalculateAnswerScore(value calculation.ScorableValue, options []questionnaire.Option) float64

	// CalculateAnswerSheetScore 计算整个答卷的得分
	CalculateAnswerSheetScore(ctx context.Context, sheet *AnswerSheet, qnr *questionnaire.Questionnaire) (*ScoredAnswerSheet, error)
}

// ==================== 计分结果值对象 ====================

// ScoredAnswerSheet 已计分的答卷
type ScoredAnswerSheet struct {
	AnswerSheetID uint64
	TotalScore    float64
	ScoredAnswers []ScoredAnswer
}

// ScoredAnswer 已计分的答案
type ScoredAnswer struct {
	QuestionCode string
	Score        float64
	MaxScore     float64
}

// ==================== 计分服务实现 ====================

type scoringService struct{}

// NewScoringService 创建计分服务
func NewScoringService() ScoringService {
	return &scoringService{}
}

// CalculateAnswerScore 计算单个答案的得分
// 设计：领域层只负责组装，将计算委托给 pkg/calculation
func (s *scoringService) CalculateAnswerScore(value calculation.ScorableValue, options []questionnaire.Option) float64 {
	if value == nil || value.IsEmpty() || len(options) == 0 {
		return 0
	}

	// 组装选项分数映射（领域层职责：数据转换）
	optionScoreMap := buildOptionScoreMap(options)

	// 委托给 calculation 层进行计算
	return calculation.Score(value, optionScoreMap)
}

// CalculateAnswerSheetScore 计算整个答卷的得分
// 设计：领域层负责组装任务，委托给 pkg/calculation 批量计算
func (s *scoringService) CalculateAnswerSheetScore(ctx context.Context, sheet *AnswerSheet, qnr *questionnaire.Questionnaire) (*ScoredAnswerSheet, error) {
	// 1. 构建问题映射（领域层职责：数据准备）
	questionMap := buildQuestionMap(qnr.GetQuestions())

	// 2. 组装批量计分任务（领域层职责：任务组装）
	tasks := s.buildScoreTasks(sheet.Answers(), questionMap)

	// 3. 委托给 calculation 层批量计算
	resultMap := calculation.BatchScoreToMap(tasks)

	// 4. 转换计算结果为领域对象（领域层职责：结果映射）
	return s.buildScoredAnswerSheet(sheet.ID(), sheet.Answers(), resultMap, questionMap), nil
}

// buildScoreTasks 组装批量计分任务
func (s *scoringService) buildScoreTasks(answers []Answer, questionMap map[string]questionnaire.Question) []calculation.ScoreTask {
	tasks := make([]calculation.ScoreTask, 0, len(answers))

	for _, ans := range answers {
		question, found := questionMap[ans.QuestionCode()]
		if !found {
			continue // 跳过找不到问题定义的答案
		}

		optionScoreMap := buildOptionScoreMap(question.GetOptions())

		tasks = append(tasks, calculation.ScoreTask{
			ID:           ans.QuestionCode(),
			Value:        NewScorableValue(ans.Value()),
			OptionScores: optionScoreMap,
		})
	}

	return tasks
}

// buildScoredAnswerSheet 构建计分结果
func (s *scoringService) buildScoredAnswerSheet(
	sheetID meta.ID,
	answers []Answer,
	resultMap map[string]calculation.ScoreResult,
	questionMap map[string]questionnaire.Question,
) *ScoredAnswerSheet {
	scoredAnswers := make([]ScoredAnswer, 0, len(answers))
	var totalScore float64

	for _, ans := range answers {
		result, found := resultMap[ans.QuestionCode()]
		if !found {
			continue
		}

		scoredAnswers = append(scoredAnswers, ScoredAnswer{
			QuestionCode: ans.QuestionCode(),
			Score:        result.Score,
			MaxScore:     result.MaxScore,
		})

		totalScore += result.Score
	}

	return &ScoredAnswerSheet{
		AnswerSheetID: uint64(sheetID),
		TotalScore:    totalScore,
		ScoredAnswers: scoredAnswers,
	}
}

// ==================== 辅助函数 ====================

// buildQuestionMap 构建问题映射
func buildQuestionMap(questions []questionnaire.Question) map[string]questionnaire.Question {
	questionMap := make(map[string]questionnaire.Question, len(questions))
	for _, q := range questions {
		questionMap[q.GetCode().Value()] = q
	}
	return questionMap
}

// buildOptionScoreMap 构建选项分数映射
func buildOptionScoreMap(options []questionnaire.Option) map[string]float64 {
	optionScoreMap := make(map[string]float64, len(options))
	for _, opt := range options {
		optionScoreMap[opt.GetCode().Value()] = opt.GetScore()
	}
	return optionScoreMap
}

// ==================== 设计说明 ====================

// 本领域服务遵循 DDD 分层原则：
// - 领域层（本文件）：负责数据组装和结果映射
// - 计算层（pkg/calculation）：负责无状态的通用计算逻辑
//
// 这种设计的优势：
// 1. 计算逻辑可复用：不同领域都可以使用 pkg/calculation
// 2. 职责清晰：领域层关注业务概念，计算层关注算法实现
// 3. 易于测试：计算层可独立进行单元测试
// 4. 支持扩展：可以通过 BatchScoreConcurrent 支持大规模并发计算
