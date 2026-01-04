package questionnaire

import (
	"context"
	"time"

	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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
	QRCodeURL   string           // 小程序码URL（仅已发布状态时返回）
}

// QuestionResult 问题结果
type QuestionResult struct {
	Code           string                              // 问题编码
	Stem           string                              // 题干
	Type           string                              // 问题类型
	Options        []OptionResult                      // 选项列表
	Required       bool                                // 是否必填
	Description    string                              // 问题描述
	ShowController *domainQuestionnaire.ShowController // 显示控制器
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
	Code          string    // 问卷编码
	Version       string    // 版本号
	Title         string    // 问卷标题
	Description   string    // 问卷描述
	ImgUrl        string    // 封面图URL
	Status        string    // 状态
	Type          string    // 问卷分类
	QuestionCount int       // 问题数量
	CreatedBy     string    // 创建人
	CreatedAt     time.Time // 创建时间
	UpdatedBy     string    // 更新人
	UpdatedAt     time.Time // 更新时间
}

// QuestionnaireSummaryListResult 问卷摘要列表结果
type QuestionnaireSummaryListResult struct {
	Items []*QuestionnaireSummaryResult // 问卷摘要列表
	Total int64                         // 总数
}

// ============= Converter 转换器 =============

// toQuestionnaireResult 将领域模型转换为结果对象
func toQuestionnaireResult(q *domainQuestionnaire.Questionnaire) *QuestionnaireResult {
	if q == nil {
		return nil
	}

	result := &QuestionnaireResult{
		Code:        q.GetCode().String(),
		Version:     q.GetVersion().String(),
		Title:       q.GetTitle(),
		Description: q.GetDescription(),
		ImgUrl:      q.GetImgUrl(),
		Status:      q.GetStatus().String(),
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
func toQuestionResult(q domainQuestionnaire.Question) QuestionResult {
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

// toQuestionnaireSummaryResult 将问卷领域模型转换为摘要结果对象
func toQuestionnaireSummaryResult(q *domainQuestionnaire.Questionnaire, userNames map[string]string) *QuestionnaireSummaryResult {
	if q == nil {
		return nil
	}

	return &QuestionnaireSummaryResult{
		Code:          q.GetCode().String(),
		Version:       q.GetVersion().String(),
		Title:         q.GetTitle(),
		Description:   q.GetDescription(),
		ImgUrl:        q.GetImgUrl(),
		Status:        q.GetStatus().String(),
		Type:          q.GetType().String(),
		QuestionCount: q.GetQuestionCnt(),
		CreatedBy:     iam.DisplayName(q.GetCreatedBy(), userNames),
		CreatedAt:     q.GetCreatedAt(),
		UpdatedBy:     iam.DisplayName(q.GetUpdatedBy(), userNames),
		UpdatedAt:     q.GetUpdatedAt(),
	}
}

// toQuestionnaireSummaryListResult 将问卷摘要列表转换为结果对象
func toQuestionnaireSummaryListResult(ctx context.Context, items []*domainQuestionnaire.Questionnaire, total int64, identitySvc *iam.IdentityService) *QuestionnaireSummaryListResult {
	userNames := resolveUserNames(ctx, items, identitySvc)
	result := &QuestionnaireSummaryListResult{
		Items: make([]*QuestionnaireSummaryResult, 0, len(items)),
		Total: total,
	}

	for _, item := range items {
		result.Items = append(result.Items, toQuestionnaireSummaryResult(item, userNames))
	}

	return result
}

func resolveUserNames(ctx context.Context, items []*domainQuestionnaire.Questionnaire, identitySvc *iam.IdentityService) map[string]string {
	userIDs := make([]meta.ID, 0, len(items)*2)
	for _, item := range items {
		if item == nil {
			continue
		}
		userIDs = append(userIDs, item.GetCreatedBy(), item.GetUpdatedBy())
	}
	return iam.ResolveUserNames(ctx, identitySvc, userIDs)
}
