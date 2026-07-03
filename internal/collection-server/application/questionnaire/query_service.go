package questionnaire

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/catalogreadthrough"
	"github.com/FangcunMount/qs-server/internal/collection-server/port/grpcbridge"
	"github.com/FangcunMount/qs-server/internal/pkg/cancelerr"
	"golang.org/x/sync/singleflight"
)

type questionnaireClient = grpcbridge.QuestionnaireReader

// QueryService 问卷查询服务
type QueryService struct {
	client            questionnaireClient
	cache             PublishedDetailCache
	singleflightGroup singleflight.Group
	useSingleflight   bool
}

// NewQueryService 创建问卷查询服务。
func NewQueryService(
	client questionnaireClient,
	cache PublishedDetailCache,
	useSingleflight bool,
) *QueryService {
	return &QueryService{
		client:          client,
		cache:           cache,
		useSingleflight: useSingleflight,
	}
}

// HasCachedDetail 进程内 L1 是否已有已发布问卷详情。
func (s *QueryService) HasCachedDetail(code, version string) bool {
	if s == nil || s.cache == nil || code == "" {
		return false
	}
	_, ok := s.cache.Get(code, version)
	return ok
}

// Get 获取问卷详情
func (s *QueryService) Get(ctx context.Context, code, version string) (*QuestionnaireResponse, error) {
	var setFn func(*QuestionnaireResponse)
	if s.cache != nil {
		setFn = func(resp *QuestionnaireResponse) { s.cache.Set(code, version, resp) }
	}
	return catalogreadthrough.ReadThrough(
		cacheKey(code, version),
		func() (*QuestionnaireResponse, bool) {
			if s.cache == nil {
				return nil, false
			}
			return s.cache.Get(code, version)
		},
		setFn,
		func() (*QuestionnaireResponse, error) { return s.fetchFromGRPC(ctx, code, version) },
		cloneResponse,
		&s.singleflightGroup,
		s.cache != nil && s.useSingleflight,
	)
}

func (s *QueryService) fetchFromGRPC(ctx context.Context, code, version string) (*QuestionnaireResponse, error) {
	log.Infof("Getting questionnaire: code=%s version=%s", code, version)

	result, err := s.client.GetQuestionnaire(ctx, code, version)
	if err != nil {
		logQuestionnaireGRPCError("Failed to get questionnaire via gRPC", err)
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

	result, err := s.client.ListQuestionnaires(ctx, req.Page, req.PageSize, req.Status, req.Title)
	if err != nil {
		logQuestionnaireGRPCError("Failed to list questionnaires via gRPC", err)
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
			Type:          q.Type,
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

func logQuestionnaireGRPCError(message string, err error) {
	if cancelerr.Is(err) {
		log.Debugf("%s: %v", message, err)
		return
	}
	log.Errorf("%s: %v", message, err)
}

// convertQuestionnaire 转换问卷
func (s *QueryService) convertQuestionnaire(q *grpcbridge.QuestionnaireOutput) *QuestionnaireResponse {
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
		Type:        q.Type,
		Questions:   questions,
		CreatedAt:   q.CreatedAt,
		UpdatedAt:   q.UpdatedAt,
	}
}

// convertQuestion 转换问题
func (s *QueryService) convertQuestion(q *grpcbridge.QuestionOutput) QuestionResponse {
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
