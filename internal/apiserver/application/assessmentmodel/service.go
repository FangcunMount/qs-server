package assessmentmodel

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/personalitymodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type Service interface {
	List(ctx context.Context, dto ListModelsDTO) (*ModelListResult, error)
	Create(ctx context.Context, dto CreateModelDTO) (*ModelSummary, error)
	Get(ctx context.Context, code string) (*ModelSummary, error)
	UpdateBasicInfo(ctx context.Context, dto UpdateBasicInfoDTO) (*ModelSummary, error)
	Delete(ctx context.Context, code string) error
	Publish(ctx context.Context, code string) (*ModelSummary, error)
	Unpublish(ctx context.Context, code string) (*ModelSummary, error)
	Archive(ctx context.Context, code string) (*ModelSummary, error)
	BindQuestionnaire(ctx context.Context, dto BindQuestionnaireDTO) (*QuestionnaireBindingResult, error)
	GetQuestionnaire(ctx context.Context, code string) (*QuestionnaireBindingResult, error)
	GetDefinition(ctx context.Context, code string) (*DefinitionDTO, error)
	UpdateDefinition(ctx context.Context, code string, dto DefinitionDTO) (*DefinitionDTO, error)
	Options(ctx context.Context, kind string) (*OptionsResult, error)
	ApplyCodes(ctx context.Context, dto ApplyCodesDTO) ([]string, error)
	Validate(ctx context.Context, code string) (*ValidationResult, error)
	PreviewReport(ctx context.Context, code string, payload json.RawMessage) (*PreviewReportResult, error)
	GetQRCode(ctx context.Context, code string) (string, error)
}

type Dependencies struct {
	ScaleLifecycle     scale.ScaleLifecycleService
	ScaleFactor        scale.ScaleFactorService
	ScaleQuery         scale.ScaleQueryService
	ScaleCategory      scale.ScaleCategoryService
	ScaleQRCode        scale.ScaleQRCodeQueryService
	PersonalityQuery   personalitymodel.PersonalityModelQueryService
	QuestionnaireQuery questionnaireapp.QuestionnaireQueryService
	Codes              codes.CodesService
	RawQRCodeGenerator qrcode.QRCodeService
}

type service struct {
	deps Dependencies
}

func NewService(deps Dependencies) Service {
	return &service{deps: deps}
}

func (s *service) List(ctx context.Context, dto ListModelsDTO) (*ModelListResult, error) {
	if dto.Page <= 0 {
		dto.Page = 1
	}
	if dto.PageSize <= 0 {
		dto.PageSize = 20
	}
	if dto.Kind != "" && dto.Kind != KindBehaviorAbility && dto.Kind != KindPersonality {
		return nil, invalidArgument("模型类型无效")
	}

	result := &ModelListResult{Page: dto.Page, PageSize: dto.PageSize}
	if dto.Kind == "" || dto.Kind == KindBehaviorAbility {
		scales, err := s.listBehaviorAbility(ctx, dto)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, scales.Items...)
		result.Total += scales.Total
	}
	if dto.Kind == "" || dto.Kind == KindPersonality {
		items, total, err := s.listPersonality(ctx, dto)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, items...)
		result.Total += total
	}
	return result, nil
}

func (s *service) Create(ctx context.Context, dto CreateModelDTO) (*ModelSummary, error) {
	if dto.Kind == "" {
		dto.Kind = KindBehaviorAbility
	}
	if dto.Kind != KindBehaviorAbility {
		return nil, unsupportedPersonalityWrite()
	}
	if s.deps.ScaleLifecycle == nil {
		return nil, unavailable("行为能力模型服务未配置")
	}
	result, err := s.deps.ScaleLifecycle.Create(ctx, scale.CreateScaleDTO{
		Code:                 dto.Code,
		Title:                dto.Title,
		Description:          dto.Description,
		Category:             dto.Category,
		Tags:                 dto.Tags,
		QuestionnaireCode:    dto.QuestionnaireCode,
		QuestionnaireVersion: dto.QuestionnaireVersion,
	})
	if err != nil {
		return nil, err
	}
	return behaviorSummaryFromScale(result), nil
}

func (s *service) Get(ctx context.Context, modelCode string) (*ModelSummary, error) {
	if modelCode == "" {
		return nil, invalidArgument("模型编码不能为空")
	}
	if s.deps.ScaleQuery != nil {
		if result, err := s.deps.ScaleQuery.GetByCode(ctx, modelCode); err == nil && result != nil {
			return behaviorSummaryFromScale(result), nil
		}
	}
	if s.deps.PersonalityQuery != nil {
		if result, err := s.deps.PersonalityQuery.GetPublishedByCode(ctx, modelCode); err == nil && result != nil {
			return personalitySummaryFromDetail(result), nil
		}
	}
	return nil, errors.WithCode(code.ErrMedicalScaleNotFound, "测评模型不存在")
}

func (s *service) UpdateBasicInfo(ctx context.Context, dto UpdateBasicInfoDTO) (*ModelSummary, error) {
	if s.deps.ScaleLifecycle == nil {
		return nil, unavailable("行为能力模型服务未配置")
	}
	result, err := s.deps.ScaleLifecycle.UpdateBasicInfo(ctx, scale.UpdateScaleBasicInfoDTO{
		Code:        dto.Code,
		Title:       dto.Title,
		Description: dto.Description,
		Category:    dto.Category,
		Tags:        dto.Tags,
	})
	if err != nil {
		return nil, err
	}
	return behaviorSummaryFromScale(result), nil
}

func (s *service) Delete(ctx context.Context, modelCode string) error {
	if s.deps.ScaleLifecycle == nil {
		return unavailable("行为能力模型服务未配置")
	}
	return s.deps.ScaleLifecycle.Delete(ctx, modelCode)
}

func (s *service) Publish(ctx context.Context, modelCode string) (*ModelSummary, error) {
	if s.deps.ScaleLifecycle == nil {
		return nil, unavailable("行为能力模型服务未配置")
	}
	result, err := s.deps.ScaleLifecycle.Publish(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return behaviorSummaryFromScale(result), nil
}

func (s *service) Unpublish(ctx context.Context, modelCode string) (*ModelSummary, error) {
	if s.deps.ScaleLifecycle == nil {
		return nil, unavailable("行为能力模型服务未配置")
	}
	result, err := s.deps.ScaleLifecycle.Unpublish(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return behaviorSummaryFromScale(result), nil
}

func (s *service) Archive(ctx context.Context, modelCode string) (*ModelSummary, error) {
	if s.deps.ScaleLifecycle == nil {
		return nil, unavailable("行为能力模型服务未配置")
	}
	result, err := s.deps.ScaleLifecycle.Archive(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return behaviorSummaryFromScale(result), nil
}

func (s *service) BindQuestionnaire(ctx context.Context, dto BindQuestionnaireDTO) (*QuestionnaireBindingResult, error) {
	if s.deps.ScaleLifecycle == nil {
		return nil, unavailable("行为能力模型服务未配置")
	}
	result, err := s.deps.ScaleLifecycle.UpdateQuestionnaire(ctx, scale.UpdateScaleQuestionnaireDTO{
		Code:                 dto.Code,
		QuestionnaireCode:    dto.QuestionnaireCode,
		QuestionnaireVersion: dto.QuestionnaireVersion,
	})
	if err != nil {
		return nil, err
	}
	return s.questionnaireBinding(ctx, result.QuestionnaireCode, result.QuestionnaireVersion)
}

func (s *service) GetQuestionnaire(ctx context.Context, modelCode string) (*QuestionnaireBindingResult, error) {
	result, err := s.loadBehaviorAbility(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return s.questionnaireBinding(ctx, result.QuestionnaireCode, result.QuestionnaireVersion)
}

func (s *service) GetDefinition(ctx context.Context, modelCode string) (*DefinitionDTO, error) {
	result, err := s.loadBehaviorAbility(ctx, modelCode)
	if err == nil {
		payload, err := json.Marshal(newBehaviorDefinitionPayload(result))
		if err != nil {
			return nil, err
		}
		return &DefinitionDTO{
			Kind:          KindBehaviorAbility,
			Algorithm:     "score_range",
			PayloadFormat: PayloadFormatScaleV1,
			Payload:       payload,
		}, nil
	}
	if s.deps.PersonalityQuery == nil {
		return nil, err
	}
	personality, personalityErr := s.deps.PersonalityQuery.GetPublishedByCode(ctx, modelCode)
	if personalityErr != nil {
		return nil, err
	}
	payload, marshalErr := json.Marshal(newPersonalityDefinitionPayload(personality))
	if marshalErr != nil {
		return nil, marshalErr
	}
	return &DefinitionDTO{
		Kind:          KindPersonality,
		SubKind:       "typology",
		Algorithm:     personality.Algorithm,
		PayloadFormat: "assessmentmodel.personality.typology.v1",
		Payload:       payload,
	}, nil
}

func (s *service) UpdateDefinition(ctx context.Context, modelCode string, dto DefinitionDTO) (*DefinitionDTO, error) {
	if dto.Kind == "" {
		dto.Kind = KindBehaviorAbility
	}
	if dto.Kind != KindBehaviorAbility {
		return nil, unsupportedPersonalityWrite()
	}
	var payload struct {
		Dimensions     []behaviorDimensionRule `json:"dimensions"`
		InterpretRules []behaviorInterpretRule `json:"interpret_rules"`
	}
	if len(dto.Payload) > 0 {
		if err := json.Unmarshal(dto.Payload, &payload); err != nil {
			return nil, invalidArgument("模型定义 payload 格式无效")
		}
	}
	if len(payload.Dimensions) == 0 {
		return nil, invalidArgument("行为能力模型维度不能为空")
	}
	if s.deps.ScaleFactor == nil {
		return nil, unavailable("行为能力模型定义服务未配置")
	}

	ruleByDimension := make(map[string][]scale.InterpretRuleDTO)
	for _, group := range payload.InterpretRules {
		for _, r := range group.Ranges {
			ruleByDimension[group.DimensionCode] = append(ruleByDimension[group.DimensionCode], scale.InterpretRuleDTO{
				MinScore:   r.MinScore,
				MaxScore:   r.MaxScore,
				RiskLevel:  r.Level,
				Conclusion: r.Conclusion,
				Suggestion: r.Suggestion,
			})
		}
	}
	factors := make([]scale.FactorDTO, 0, len(payload.Dimensions))
	for _, d := range payload.Dimensions {
		if d.Code == "" || d.Title == "" {
			return nil, invalidArgument("维度编码和标题不能为空")
		}
		factors = append(factors, scale.FactorDTO{
			Code:            d.Code,
			Title:           d.Title,
			FactorType:      "primary",
			QuestionCodes:   d.QuestionCodes,
			ScoringStrategy: d.ScoringStrategy,
			ScoringParams:   scoringParamsDTO(d.ScoringParams),
			MaxScore:        d.MaxScore,
			IsTotalScore:    d.IsTotalScore,
			IsShow:          d.IsShow,
			InterpretRules:  ruleByDimension[d.Code],
		})
	}
	if _, err := s.deps.ScaleFactor.ReplaceFactors(ctx, modelCode, factors); err != nil {
		return nil, err
	}
	return s.GetDefinition(ctx, modelCode)
}

func (s *service) Options(ctx context.Context, kind string) (*OptionsResult, error) {
	result := &OptionsResult{
		Kinds: []Option{
			{Label: "人格测评", Value: KindPersonality},
			{Label: "行为能力测评", Value: KindBehaviorAbility},
		},
		Algorithms: []Option{
			{Label: "人格类型", Value: "personality_typology"},
			{Label: "分数区间解释", Value: "score_range"},
		},
		SubKinds: []Option{
			{Label: "类型人格", Value: "typology"},
			{Label: "维度评分", Value: "dimension_score"},
		},
	}
	if kind == "" || kind == KindBehaviorAbility {
		if s.deps.ScaleCategory != nil {
			categories, err := s.deps.ScaleCategory.GetCategories(ctx)
			if err != nil {
				return nil, err
			}
			for _, item := range categories.Categories {
				result.Categories = append(result.Categories, Option{Label: item.Label, Value: item.Value})
			}
			for _, item := range categories.Tags {
				result.Tags = append(result.Tags, Option{Label: item.Label, Value: item.Value})
			}
		}
	}
	return result, nil
}

func (s *service) ApplyCodes(ctx context.Context, dto ApplyCodesDTO) ([]string, error) {
	if s.deps.Codes == nil {
		return nil, unavailable("编码服务未配置")
	}
	kind, prefix := codeKindAndPrefix(dto.Target)
	if kind == "" {
		return nil, invalidArgument("编码申请目标无效")
	}
	return s.deps.Codes.Apply(ctx, kind, dto.Count, prefix, map[string]interface{}{"assessment_model_code": dto.Code, "target": dto.Target})
}

func (s *service) Validate(ctx context.Context, modelCode string) (*ValidationResult, error) {
	def, err := s.GetDefinition(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	result := &ValidationResult{Valid: true}
	if len(def.Payload) == 0 {
		result.Valid = false
		result.Errors = append(result.Errors, "模型定义 payload 不能为空")
	}
	return result, nil
}

func (s *service) PreviewReport(_ context.Context, _ string, _ json.RawMessage) (*PreviewReportResult, error) {
	return nil, errors.WithCode(code.ErrInvalidArgument, "预览报告生成尚未接入统一测评模型后台接口")
}

func (s *service) GetQRCode(ctx context.Context, modelCode string) (string, error) {
	if s.deps.ScaleQRCode == nil {
		return "", unavailable("模型二维码服务未配置")
	}
	return s.deps.ScaleQRCode.GetQRCode(ctx, modelCode)
}

func (s *service) listBehaviorAbility(ctx context.Context, dto ListModelsDTO) (*ModelListResult, error) {
	if s.deps.ScaleQuery == nil {
		return &ModelListResult{Page: dto.Page, PageSize: dto.PageSize}, nil
	}
	result, err := s.deps.ScaleQuery.List(ctx, scale.ListScalesDTO{
		Page:     dto.Page,
		PageSize: dto.PageSize,
		Filter: scale.ScaleListFilter{
			Status:   dto.Status,
			Title:    dto.Keyword,
			Category: dto.Category,
		},
	})
	if err != nil {
		return nil, err
	}
	out := &ModelListResult{Page: dto.Page, PageSize: dto.PageSize}
	if result == nil {
		return out, nil
	}
	out.Total = result.Total
	for _, item := range result.Items {
		out.Items = append(out.Items, behaviorSummaryFromScaleSummary(item))
	}
	return out, nil
}

func (s *service) listPersonality(ctx context.Context, dto ListModelsDTO) ([]ModelSummary, int64, error) {
	if s.deps.PersonalityQuery == nil {
		return nil, 0, nil
	}
	if dto.Status != "" && dto.Status != StatusPublished {
		return nil, 0, nil
	}
	result, err := s.deps.PersonalityQuery.ListPublished(ctx, personalitymodel.ListPersonalityModelsDTO{
		Page:     dto.Page,
		PageSize: dto.PageSize,
	})
	if err != nil {
		return nil, 0, err
	}
	items := make([]ModelSummary, 0, len(result.Items))
	for _, item := range result.Items {
		if dto.Keyword != "" && item.Title != "" && !containsFold(item.Title, dto.Keyword) {
			continue
		}
		items = append(items, personalitySummaryFromSummary(item))
	}
	return items, result.Total, nil
}

func (s *service) loadBehaviorAbility(ctx context.Context, modelCode string) (*scale.ScaleResult, error) {
	if modelCode == "" {
		return nil, invalidArgument("模型编码不能为空")
	}
	if s.deps.ScaleQuery == nil {
		return nil, unavailable("行为能力模型服务未配置")
	}
	return s.deps.ScaleQuery.GetByCode(ctx, modelCode)
}

func (s *service) questionnaireBinding(ctx context.Context, questionnaireCode, questionnaireVersion string) (*QuestionnaireBindingResult, error) {
	result := &QuestionnaireBindingResult{
		QuestionnaireCode:    questionnaireCode,
		QuestionnaireVersion: questionnaireVersion,
	}
	if questionnaireCode == "" || s.deps.QuestionnaireQuery == nil {
		return result, nil
	}
	var q *questionnaireapp.QuestionnaireResult
	var err error
	if questionnaireVersion != "" {
		q, err = s.deps.QuestionnaireQuery.GetPublishedByCodeVersion(ctx, questionnaireCode, questionnaireVersion)
	} else {
		q, err = s.deps.QuestionnaireQuery.GetByCode(ctx, questionnaireCode)
	}
	if err != nil {
		return result, nil
	}
	if q != nil {
		result.Title = q.Title
		result.QuestionCount = len(q.Questions)
	}
	return result, nil
}

func invalidArgument(format string, args ...interface{}) error {
	return errors.WithCode(code.ErrInvalidArgument, format, args...)
}

func unavailable(message string) error {
	return errors.WithCode(code.ErrInternalServerError, "%s", message)
}

func unsupportedPersonalityWrite() error {
	return errors.WithCode(code.ErrInvalidArgument, "人格测评后台编辑尚未接入可写仓储，请先使用行为能力模型或等待人格模型写侧接入")
}

func codeKindAndPrefix(target string) (string, string) {
	switch target {
	case "dimension":
		return "factor", "dim"
	case "outcome":
		return "outcome", "out"
	case "rule":
		return "rule", "rule"
	default:
		return "", ""
	}
}

func scoringParamsDTO(params map[string]interface{}) *scale.ScoringParamsDTO {
	if len(params) == 0 {
		return nil
	}
	dto := &scale.ScoringParamsDTO{}
	if raw, ok := params["cnt_option_contents"].([]interface{}); ok {
		for _, item := range raw {
			dto.CntOptionContents = append(dto.CntOptionContents, fmt.Sprint(item))
		}
	}
	if raw, ok := params["cnt_option_contents"].([]string); ok {
		dto.CntOptionContents = append(dto.CntOptionContents, raw...)
	}
	return dto
}
