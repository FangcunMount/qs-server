package questionnaire

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
)

// QueryService 问卷查询服务
// 作为 BFF 层的薄服务，主要职责：
// 1. 调用 apiserver 的 gRPC 服务
// 2. 转换 gRPC 响应到 REST DTO
// 3. 可选：缓存热点数据
type QueryService struct {
	questionnaireClient *grpcclient.QuestionnaireClient
}

// NewQueryService 创建问卷查询服务
func NewQueryService(
	questionnaireClient *grpcclient.QuestionnaireClient,
) *QueryService {
	return &QueryService{
		questionnaireClient: questionnaireClient,
	}
}

// Get 获取问卷详情
func (s *QueryService) Get(ctx context.Context, code string) (*QuestionnaireResponse, error) {
	log.Infof("Getting questionnaire: code=%s", code)

	result, err := s.questionnaireClient.GetQuestionnaire(ctx, code)
	if err != nil {
		log.Errorf("Failed to get questionnaire via gRPC: %v", err)
		return nil, err
	}

	if result == nil {
		return nil, nil
	}

	return s.convertQuestionnaire(result), nil
}

// List 获取问卷列表（返回摘要，不含问题详情）
func (s *QueryService) List(ctx context.Context, req *ListQuestionnairesRequest) (*ListQuestionnairesResponse, error) {
	log.Infof("Listing questionnaires: page=%d, pageSize=%d, status=%s", req.Page, req.PageSize, req.Status)

	// 默认分页参数
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 50
	}
	// 最大分页限制，避免一次查询过多数据
	if req.PageSize > 100 {
		req.PageSize = 100
	}

	result, err := s.questionnaireClient.ListQuestionnaires(ctx, req.Page, req.PageSize, req.Status, req.Title)
	if err != nil {
		log.Errorf("Failed to list questionnaires via gRPC: %v", err)
		return nil, err
	}

	// 直接使用摘要类型，不需要转换完整问卷
	questionnaires := make([]QuestionnaireSummaryResponse, len(result.Questionnaires))
	for i, q := range result.Questionnaires {
		questionnaires[i] = QuestionnaireSummaryResponse{
			Code:          q.Code,
			Title:         q.Title,
			Description:   q.Description,
			ImgURL:        q.ImgURL,
			Status:        q.Status,
			Version:       q.Version,
			QuestionCount: q.QuestionCount,
			CreatedAt:     q.CreatedAt,
			UpdatedAt:     q.UpdatedAt,
		}
	}

	return &ListQuestionnairesResponse{
		Questionnaires: questionnaires,
		Total:          result.Total,
		Page:           result.Page,
		PageSize:       result.PageSize,
	}, nil
}

// convertQuestionnaire 转换问卷
func (s *QueryService) convertQuestionnaire(q *grpcclient.QuestionnaireOutput) *QuestionnaireResponse {
	questions := make([]QuestionResponse, len(q.Questions))
	for i, question := range q.Questions {
		questions[i] = s.convertQuestion(&question)
	}

	return &QuestionnaireResponse{
		Code:        q.Code,
		Title:       q.Title,
		Description: q.Description,
		ImgURL:      q.ImgURL,
		Status:      q.Status,
		Version:     q.Version,
		Questions:   questions,
		CreatedAt:   q.CreatedAt,
		UpdatedAt:   q.UpdatedAt,
	}
}

// convertQuestion 转换问题
func (s *QueryService) convertQuestion(q *grpcclient.QuestionOutput) QuestionResponse {
	options := make([]OptionResponse, len(q.Options))
	for i, opt := range q.Options {
		options[i] = OptionResponse{
			Code:    opt.Code,
			Content: opt.Content,
			Score:   opt.Score,
		}
	}

	validationRules := make([]ValidationRuleResponse, len(q.ValidationRules))
	for i, rule := range q.ValidationRules {
		validationRules[i] = ValidationRuleResponse{
			RuleType:    rule.RuleType,
			TargetValue: rule.TargetValue,
		}
	}

	var calcRule *CalculationRuleResponse
	if q.CalculationRule != nil {
		calcRule = &CalculationRuleResponse{
			FormulaType: q.CalculationRule.FormulaType,
		}
	}

	return QuestionResponse{
		Code:            q.Code,
		Type:            q.Type,
		Title:           q.Title,
		Tips:            q.Tips,
		Placeholder:     q.Placeholder,
		Options:         options,
		ValidationRules: validationRules,
		CalculationRule: calcRule,
	}
}
