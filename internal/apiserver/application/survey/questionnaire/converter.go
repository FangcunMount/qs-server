package questionnaire

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
)

// ============= Result 定义 =============
// Results 用于应用服务层的输出结果

// QuestionnaireResult 问卷结果
type QuestionnaireResult struct {
	Code        string           // 问卷编码
	Version     string           // 版本号
	Title       string           // 问卷标题
	Description string           // 问卷描述
	ImgUrl      string           // 封面图URL
	Status      string           // 状态
	Type        string           // 问卷分类
	Questions   []QuestionResult // 问题列表
}

// QuestionResult 问题结果
type QuestionResult struct {
	Code           string                      // 问题编码
	Stem           string                      // 题干
	Type           string                      // 问题类型
	Options        []OptionResult              // 选项列表
	Required       bool                        // 是否必填
	Description    string                      // 问题描述
	ShowController *questionnaire.ShowController // 显示控制器
}

// OptionResult 选项结果
type OptionResult struct {
	Label string // 选项标签
	Value string // 选项值
	Score int    // 选项分数
}

// QuestionnaireListResult 问卷列表结果
type QuestionnaireListResult struct {
	Items []*QuestionnaireResult // 问卷列表
	Total int64                  // 总数
}

// QuestionnaireSummaryResult 问卷摘要结果（轻量级，不包含问题详情）
type QuestionnaireSummaryResult struct {
	Code          string // 问卷编码
	Version       string // 版本号
	Title         string // 问卷标题
	Description   string // 问卷描述
	ImgUrl        string // 封面图URL
	Status        string // 状态
	Type          string // 问卷分类
	QuestionCount int    // 问题数量
}

// QuestionnaireSummaryListResult 问卷摘要列表结果
type QuestionnaireSummaryListResult struct {
	Items []*QuestionnaireSummaryResult // 问卷摘要列表
	Total int64                         // 总数
}

// ============= Converter 转换器 =============

// toQuestionnaireResult 将领域模型转换为结果对象
func toQuestionnaireResult(q *questionnaire.Questionnaire) *QuestionnaireResult {
	if q == nil {
		return nil
	}

	result := &QuestionnaireResult{
		Code:        q.GetCode().String(),
		Version:     q.GetVersion().String(),
		Title:       q.GetTitle(),
		Description: q.GetDescription(),
		ImgUrl:      q.GetImgUrl(),
		Status:      string(q.GetStatus()),
		Type:        q.GetType().String(),
		Questions:   make([]QuestionResult, 0),
	}

	// 转换问题列表
	for _, question := range q.GetQuestions() {
		result.Questions = append(result.Questions, toQuestionResult(question))
	}

	return result
}

// toQuestionResult 将问题领域模型转换为结果对象
func toQuestionResult(q questionnaire.Question) QuestionResult {
	result := QuestionResult{
		Code:        q.GetCode().String(),
		Stem:        q.GetStem(),
		Type:        string(q.GetType()),
		Required:    false, // 从验证规则中推断
		Description: q.GetTips(),
		Options:     make([]OptionResult, 0),
	}

	// 检查是否有必填验证规则
	for _, rule := range q.GetValidationRules() {
		if string(rule.GetRuleType()) == "required" {
			result.Required = true
			break
		}
	}

	// 转换选项列表（如果有）
	if opts := q.GetOptions(); opts != nil {
		for _, opt := range opts {
			result.Options = append(result.Options, OptionResult{
				Label: opt.GetContent(),
				Value: opt.GetCode().String(),
				Score: int(opt.GetScore()),
			})
		}
	}

	// 转换显示控制器（如果有）
	result.ShowController = q.GetShowController()

	return result
}

// toQuestionnaireListResult 将问卷列表转换为结果对象
func toQuestionnaireListResult(items []*questionnaire.Questionnaire, total int64) *QuestionnaireListResult {
	result := &QuestionnaireListResult{
		Items: make([]*QuestionnaireResult, 0, len(items)),
		Total: total,
	}

	for _, item := range items {
		result.Items = append(result.Items, toQuestionnaireResult(item))
	}

	return result
}

// toQuestionnaireSummaryResult 将问卷摘要领域模型转换为结果对象
func toQuestionnaireSummaryResult(s *questionnaire.QuestionnaireSummary) *QuestionnaireSummaryResult {
	if s == nil {
		return nil
	}

	return &QuestionnaireSummaryResult{
		Code:          s.Code,
		Version:       s.Version,
		Title:         s.Title,
		Description:   s.Description,
		ImgUrl:        s.ImgUrl,
		Status:        string(s.Status),
		Type:          s.Type.String(),
		QuestionCount: s.QuestionCount,
	}
}

// toQuestionnaireSummaryListResult 将问卷摘要列表转换为结果对象
func toQuestionnaireSummaryListResult(items []*questionnaire.QuestionnaireSummary, total int64) *QuestionnaireSummaryListResult {
	result := &QuestionnaireSummaryListResult{
		Items: make([]*QuestionnaireSummaryResult, 0, len(items)),
		Total: total,
	}

	for _, item := range items {
		result.Items = append(result.Items, toQuestionnaireSummaryResult(item))
	}

	return result
}
