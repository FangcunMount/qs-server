package behavior

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavior/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

const (
	KindBehaviorAbility = "behavior_ability"
	SubKindScale        = "scale"
	AlgorithmScoreRange = "score_range"
	PayloadFormatScale  = "assessmentmodel.behavior_ability.scale.v1"
)

type Command interface {
	List(ctx context.Context, input ListInput) (*ListResult, error)
	Create(ctx context.Context, input CreateInput) (*Model, error)
	Get(ctx context.Context, modelCode string) (*Model, error)
	UpdateBasicInfo(ctx context.Context, input UpdateBasicInfoInput) (*Model, error)
	Delete(ctx context.Context, modelCode string) error
	Publish(ctx context.Context, modelCode string) (*Model, error)
	Unpublish(ctx context.Context, modelCode string) (*Model, error)
	Archive(ctx context.Context, modelCode string) (*Model, error)
	BindQuestionnaire(ctx context.Context, input BindQuestionnaireInput) (*Binding, error)
	GetDefinition(ctx context.Context, modelCode string) (*Definition, error)
	UpdateDefinition(ctx context.Context, modelCode string, input DefinitionInput) (*Definition, error)
	Options(ctx context.Context) (*Options, error)
	GetQRCode(ctx context.Context, modelCode string) (string, error)
}

type LegacyScaleDeps struct {
	Lifecycle scale.ScaleLifecycleService
	Factor    scale.ScaleFactorService
	Query     scale.ScaleQueryService
	Category  scale.ScaleCategoryService
	QRCode    scale.ScaleQRCodeQueryService
}

type legacyScaleCommand struct {
	deps LegacyScaleDeps
}

func NewLegacyScaleCommand(deps LegacyScaleDeps) Command {
	return &legacyScaleCommand{deps: deps}
}

type ListInput struct {
	Page     int
	PageSize int
	Status   string
	Keyword  string
	Category string
}

type CreateInput struct {
	Code                 string
	Title                string
	Description          string
	Category             string
	Tags                 []string
	QuestionnaireCode    string
	QuestionnaireVersion string
}

type UpdateBasicInfoInput struct {
	Code        string
	Title       string
	Description string
	Category    string
	Tags        []string
}

type BindQuestionnaireInput struct {
	Code                 string
	QuestionnaireCode    string
	QuestionnaireVersion string
}

type DefinitionInput struct {
	Payload json.RawMessage
}

type Model struct {
	Code                 string
	Title                string
	Description          string
	Status               string
	Category             string
	Tags                 []string
	QuestionnaireCode    string
	QuestionnaireVersion string
	CreatedAt            string
	UpdatedAt            string
}

type ListResult struct {
	Items []Model
	Total int64
}

type Binding struct {
	QuestionnaireCode    string
	QuestionnaireVersion string
}

type Definition struct {
	Kind          string
	SubKind       string
	Algorithm     string
	PayloadFormat string
	Payload       json.RawMessage
}

type Options struct {
	Categories []Option
	Tags       []Option
}

type Option struct {
	Label string
	Value string
}

type definitionPayload struct {
	Dimensions     []dimensionRule `json:"dimensions"`
	InterpretRules []interpretRule `json:"interpret_rules"`
}

type dimensionRule struct {
	Code            string                 `json:"code"`
	Title           string                 `json:"title"`
	QuestionCodes   []string               `json:"question_codes"`
	ScoringStrategy string                 `json:"scoring_strategy"`
	ScoringParams   map[string]interface{} `json:"scoring_params,omitempty"`
	MaxScore        *float64               `json:"max_score,omitempty"`
	IsTotalScore    bool                   `json:"is_total_score,omitempty"`
	IsShow          bool                   `json:"is_show"`
}

type interpretRule struct {
	DimensionCode string       `json:"dimension_code"`
	Ranges        []scoreRange `json:"ranges"`
}

type scoreRange struct {
	MinScore   float64 `json:"min_score"`
	MaxScore   float64 `json:"max_score"`
	Conclusion string  `json:"conclusion"`
	Suggestion string  `json:"suggestion,omitempty"`
	Level      string  `json:"level,omitempty"`
}

func (c *legacyScaleCommand) List(ctx context.Context, input ListInput) (*ListResult, error) {
	if c.deps.Query == nil {
		return &ListResult{}, nil
	}
	result, err := c.deps.Query.List(ctx, scale.ListScalesDTO{
		Page:     input.Page,
		PageSize: input.PageSize,
		Filter: scale.ScaleListFilter{
			Status:   input.Status,
			Title:    input.Keyword,
			Category: input.Category,
		},
	})
	if err != nil {
		return nil, err
	}
	out := &ListResult{}
	if result == nil {
		return out, nil
	}
	out.Total = result.Total
	for _, item := range result.Items {
		if item == nil {
			continue
		}
		out.Items = append(out.Items, modelFromSummary(item))
	}
	return out, nil
}

func (c *legacyScaleCommand) Create(ctx context.Context, input CreateInput) (*Model, error) {
	if c.deps.Lifecycle == nil {
		return nil, unavailable("行为能力模型服务未配置")
	}
	result, err := c.deps.Lifecycle.Create(ctx, scale.CreateScaleDTO{
		Code:                 input.Code,
		Title:                input.Title,
		Description:          input.Description,
		Category:             input.Category,
		Tags:                 input.Tags,
		QuestionnaireCode:    input.QuestionnaireCode,
		QuestionnaireVersion: input.QuestionnaireVersion,
	})
	if err != nil {
		return nil, err
	}
	return modelFromScale(result), nil
}

func (c *legacyScaleCommand) Get(ctx context.Context, modelCode string) (*Model, error) {
	if c.deps.Query == nil {
		return nil, unavailable("行为能力模型服务未配置")
	}
	result, err := c.deps.Query.GetByCode(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return modelFromScale(result), nil
}

func (c *legacyScaleCommand) UpdateBasicInfo(ctx context.Context, input UpdateBasicInfoInput) (*Model, error) {
	if c.deps.Lifecycle == nil {
		return nil, unavailable("行为能力模型服务未配置")
	}
	result, err := c.deps.Lifecycle.UpdateBasicInfo(ctx, scale.UpdateScaleBasicInfoDTO{
		Code:        input.Code,
		Title:       input.Title,
		Description: input.Description,
		Category:    input.Category,
		Tags:        input.Tags,
	})
	if err != nil {
		return nil, err
	}
	return modelFromScale(result), nil
}

func (c *legacyScaleCommand) Delete(ctx context.Context, modelCode string) error {
	if c.deps.Lifecycle == nil {
		return unavailable("行为能力模型服务未配置")
	}
	return c.deps.Lifecycle.Delete(ctx, modelCode)
}

func (c *legacyScaleCommand) Publish(ctx context.Context, modelCode string) (*Model, error) {
	if c.deps.Lifecycle == nil {
		return nil, unavailable("行为能力模型服务未配置")
	}
	result, err := c.deps.Lifecycle.Publish(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return modelFromScale(result), nil
}

func (c *legacyScaleCommand) Unpublish(ctx context.Context, modelCode string) (*Model, error) {
	if c.deps.Lifecycle == nil {
		return nil, unavailable("行为能力模型服务未配置")
	}
	result, err := c.deps.Lifecycle.Unpublish(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return modelFromScale(result), nil
}

func (c *legacyScaleCommand) Archive(ctx context.Context, modelCode string) (*Model, error) {
	if c.deps.Lifecycle == nil {
		return nil, unavailable("行为能力模型服务未配置")
	}
	result, err := c.deps.Lifecycle.Archive(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return modelFromScale(result), nil
}

func (c *legacyScaleCommand) BindQuestionnaire(ctx context.Context, input BindQuestionnaireInput) (*Binding, error) {
	if c.deps.Lifecycle == nil {
		return nil, unavailable("行为能力模型服务未配置")
	}
	result, err := c.deps.Lifecycle.UpdateQuestionnaire(ctx, scale.UpdateScaleQuestionnaireDTO{
		Code:                 input.Code,
		QuestionnaireCode:    input.QuestionnaireCode,
		QuestionnaireVersion: input.QuestionnaireVersion,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return &Binding{}, nil
	}
	return &Binding{QuestionnaireCode: result.QuestionnaireCode, QuestionnaireVersion: result.QuestionnaireVersion}, nil
}

func (c *legacyScaleCommand) GetDefinition(ctx context.Context, modelCode string) (*Definition, error) {
	if c.deps.Query == nil {
		return nil, unavailable("行为能力模型服务未配置")
	}
	result, err := c.deps.Query.GetByCode(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(newDefinitionPayload(result))
	if err != nil {
		return nil, err
	}
	return &Definition{
		Kind:          KindBehaviorAbility,
		SubKind:       SubKindScale,
		Algorithm:     AlgorithmScoreRange,
		PayloadFormat: PayloadFormatScale,
		Payload:       payload,
	}, nil
}

func (c *legacyScaleCommand) UpdateDefinition(ctx context.Context, modelCode string, input DefinitionInput) (*Definition, error) {
	var payload definitionPayload
	if len(input.Payload) > 0 {
		if err := json.Unmarshal(input.Payload, &payload); err != nil {
			return nil, invalidArgument("模型定义 payload 格式无效")
		}
	}
	if len(payload.Dimensions) == 0 {
		return nil, invalidArgument("行为能力模型维度不能为空")
	}
	if c.deps.Factor == nil {
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
	if _, err := c.deps.Factor.ReplaceFactors(ctx, modelCode, factors); err != nil {
		return nil, err
	}
	return c.GetDefinition(ctx, modelCode)
}

func (c *legacyScaleCommand) Options(ctx context.Context) (*Options, error) {
	if c.deps.Category == nil {
		return &Options{}, nil
	}
	categories, err := c.deps.Category.GetCategories(ctx)
	if err != nil {
		return nil, err
	}
	result := &Options{}
	if categories == nil {
		return result, nil
	}
	for _, item := range categories.Categories {
		result.Categories = append(result.Categories, Option{Label: item.Label, Value: item.Value})
	}
	for _, item := range categories.Tags {
		result.Tags = append(result.Tags, Option{Label: item.Label, Value: item.Value})
	}
	return result, nil
}

func (c *legacyScaleCommand) GetQRCode(ctx context.Context, modelCode string) (string, error) {
	if c.deps.QRCode == nil {
		return "", unavailable("模型二维码服务未配置")
	}
	return c.deps.QRCode.GetQRCode(ctx, modelCode)
}

func modelFromScale(result *scale.ScaleResult) *Model {
	if result == nil {
		return nil
	}
	model := modelFromFields(
		result.Code,
		result.Title,
		result.Description,
		result.Status,
		result.Category,
		result.Tags,
		result.QuestionnaireCode,
		result.QuestionnaireVersion,
	)
	model.CreatedAt = result.CreatedAt.Format("2006-01-02 15:04:05")
	model.UpdatedAt = result.UpdatedAt.Format("2006-01-02 15:04:05")
	return &model
}

func modelFromSummary(result *scale.ScaleSummaryResult) Model {
	if result == nil {
		return Model{}
	}
	model := modelFromFields(
		result.Code,
		result.Title,
		result.Description,
		result.Status,
		result.Category,
		result.Tags,
		result.QuestionnaireCode,
		"",
	)
	model.CreatedAt = result.CreatedAt.Format("2006-01-02 15:04:05")
	model.UpdatedAt = result.UpdatedAt.Format("2006-01-02 15:04:05")
	return model
}

func modelFromFields(code, title, description, status, category string, tags []string, questionnaireCode, questionnaireVersion string) Model {
	return Model{
		Code:                 code,
		Title:                title,
		Description:          description,
		Status:               status,
		Category:             category,
		Tags:                 tags,
		QuestionnaireCode:    questionnaireCode,
		QuestionnaireVersion: questionnaireVersion,
	}
}

func newDefinitionPayload(result *scale.ScaleResult) definitionPayload {
	payload := definitionPayload{}
	if result == nil {
		return payload
	}
	payload.Dimensions = make([]dimensionRule, 0, len(result.Factors))
	payload.InterpretRules = make([]interpretRule, 0, len(result.Factors))
	for _, factor := range result.Factors {
		payload.Dimensions = append(payload.Dimensions, dimensionRule{
			Code:            factor.Code,
			Title:           factor.Title,
			QuestionCodes:   factor.QuestionCodes,
			ScoringStrategy: factor.ScoringStrategy,
			ScoringParams:   factor.ScoringParams,
			MaxScore:        factor.MaxScore,
			IsTotalScore:    factor.IsTotalScore,
			IsShow:          factor.IsShow,
		})
		rules := make([]scoreRange, 0, len(factor.InterpretRules))
		for _, rule := range factor.InterpretRules {
			rules = append(rules, scoreRange{
				MinScore:   rule.MinScore,
				MaxScore:   rule.MaxScore,
				Conclusion: rule.Conclusion,
				Suggestion: rule.Suggestion,
				Level:      rule.RiskLevel,
			})
		}
		payload.InterpretRules = append(payload.InterpretRules, interpretRule{
			DimensionCode: factor.Code,
			Ranges:        rules,
		})
	}
	return payload
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

func invalidArgument(format string, args ...interface{}) error {
	return errors.WithCode(code.ErrInvalidArgument, format, args...)
}

func unavailable(message string) error {
	return errors.WithCode(code.ErrInternalServerError, "%s", message)
}
