package modelcatalog

import (
	"context"
	"encoding/json"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavior"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/personality"
	personalitymodel "github.com/FangcunMount/qs-server/internal/apiserver/application/personalitymodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
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
	BehaviorCommand    behavior.Command
	PersonalityCommand personality.Service
	PersonalityQuery   personalitymodel.PersonalityModelQueryService
	QuestionnaireQuery questionnaireapp.QuestionnaireQueryService
	Codes              codes.CodesService
	RawQRCodeGenerator qrcode.QRCodeService
}

type service struct {
	deps        Dependencies
	behavior    behaviorGateway
	personality personalityGateway
}

func NewService(deps Dependencies) Service {
	return &service{
		deps:        deps,
		behavior:    behaviorGateway{cmd: deps.BehaviorCommand},
		personality: personalityGateway{cmd: deps.PersonalityCommand},
	}
}

func (s *service) List(ctx context.Context, dto ListModelsDTO) (*ModelListResult, error) {
	if dto.Page <= 0 {
		dto.Page = 1
	}
	if dto.PageSize <= 0 {
		dto.PageSize = 20
	}
	if dto.Kind != "" {
		if err := requireCatalogOperation(dto.Kind, domain.CatalogOpList); err != nil {
			return nil, err
		}
	}

	result := &ModelListResult{Page: dto.Page, PageSize: dto.PageSize}
	if shouldListModelKind(dto.Kind, KindBehaviorAbility) {
		scales, err := s.listBehaviorAbility(ctx, dto)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, scales.Items...)
		result.Total += scales.Total
	}
	if shouldListModelKind(dto.Kind, KindPersonality) {
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
		return nil, invalidArgument("模型类型不能为空")
	}
	if err := requireCatalogOperation(dto.Kind, domain.CatalogOpCreate); err != nil {
		return nil, err
	}
	switch dto.Kind {
	case KindBehaviorAbility:
		return s.createBehaviorAbility(ctx, dto)
	case KindPersonality:
		return s.personality.create(ctx, dto)
	default:
		return nil, invalidArgument("模型类型无效")
	}
}

func (s *service) Get(ctx context.Context, modelCode string) (*ModelSummary, error) {
	if modelCode == "" {
		return nil, invalidArgument("模型编码不能为空")
	}
	if s.behavior.cmd != nil {
		if result, err := s.behavior.cmd.Get(ctx, modelCode); err == nil && result != nil {
			return summaryFromBehavior(result), nil
		}
	}
	if s.personality.cmd != nil {
		if result, err := s.personality.cmd.Get(ctx, modelCode); err == nil && result != nil {
			return summaryFromPersonality(result), nil
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
	kind, err := s.requireModelOperation(ctx, dto.Code, "", domain.CatalogOpUpdateBasicInfo)
	if err != nil {
		return nil, err
	}
	switch kind {
	case KindPersonality:
		return s.personality.updateBasicInfo(ctx, dto)
	case KindBehaviorAbility:
		return s.updateBehaviorBasicInfo(ctx, dto)
	default:
		return nil, invalidArgument("模型类型无效")
	}
}

func (s *service) Delete(ctx context.Context, modelCode string) error {
	kind, err := s.requireModelOperation(ctx, modelCode, "", domain.CatalogOpDelete)
	if err != nil {
		return err
	}
	switch kind {
	case KindPersonality:
		return s.personality.delete(ctx, modelCode)
	case KindBehaviorAbility:
		return s.behavior.delete(ctx, modelCode)
	default:
		return invalidArgument("模型类型无效")
	}
}

func (s *service) Publish(ctx context.Context, modelCode string) (*ModelSummary, error) {
	kind, err := s.requireModelOperation(ctx, modelCode, "", domain.CatalogOpPublish)
	if err != nil {
		return nil, err
	}
	switch kind {
	case KindPersonality:
		return s.personality.publish(ctx, modelCode)
	case KindBehaviorAbility:
		return s.behavior.publish(ctx, modelCode)
	default:
		return nil, invalidArgument("模型类型无效")
	}
}

func (s *service) Unpublish(ctx context.Context, modelCode string) (*ModelSummary, error) {
	kind, err := s.requireModelOperation(ctx, modelCode, "", domain.CatalogOpUnpublish)
	if err != nil {
		return nil, err
	}
	switch kind {
	case KindPersonality:
		return s.personality.unpublish(ctx, modelCode)
	case KindBehaviorAbility:
		return s.behavior.unpublish(ctx, modelCode)
	default:
		return nil, invalidArgument("模型类型无效")
	}
}

func (s *service) Archive(ctx context.Context, modelCode string) (*ModelSummary, error) {
	kind, err := s.requireModelOperation(ctx, modelCode, "", domain.CatalogOpArchive)
	if err != nil {
		return nil, err
	}
	switch kind {
	case KindPersonality:
		return s.personality.archive(ctx, modelCode)
	case KindBehaviorAbility:
		return s.behavior.archive(ctx, modelCode)
	default:
		return nil, invalidArgument("模型类型无效")
	}
}

func (s *service) BindQuestionnaire(ctx context.Context, dto BindQuestionnaireDTO) (*QuestionnaireBindingResult, error) {
	kind, err := s.requireModelOperation(ctx, dto.Code, "", domain.CatalogOpBindQuestionnaire)
	if err != nil {
		return nil, err
	}
	switch kind {
	case KindPersonality:
		return s.personality.bindQuestionnaire(ctx, dto)
	case KindBehaviorAbility:
		binding, bindErr := s.behavior.bindQuestionnaire(ctx, dto)
		if bindErr != nil {
			return nil, bindErr
		}
		return s.questionnaireBinding(ctx, binding.QuestionnaireCode, binding.QuestionnaireVersion)
	default:
		return nil, invalidArgument("模型类型无效")
	}
}

func (s *service) GetQuestionnaire(ctx context.Context, modelCode string) (*QuestionnaireBindingResult, error) {
	if kind, ok := s.resolveModelKind(ctx, modelCode); ok && kind == KindPersonality {
		return s.personality.getQuestionnaire(ctx, modelCode)
	}
	result, err := s.loadBehaviorAbility(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return s.questionnaireBinding(ctx, result.QuestionnaireCode, result.QuestionnaireVersion)
}

func (s *service) GetDefinition(ctx context.Context, modelCode string) (*DefinitionDTO, error) {
	if kind, ok := s.resolveModelKind(ctx, modelCode); ok && kind == KindPersonality {
		return s.personality.getDefinition(ctx, modelCode)
	}
	result, err := s.loadBehaviorAbility(ctx, modelCode)
	if err == nil {
		definition, err := s.behavior.cmd.GetDefinition(ctx, result.Code)
		if err != nil {
			return nil, err
		}
		return definitionFromBehavior(definition), nil
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
		SubKind:       SubKindTypology,
		Algorithm:     personality.Algorithm,
		PayloadFormat: PayloadFormatPersonalityTypologyV1,
		Payload:       payload,
	}, nil
}

func (s *service) UpdateDefinition(ctx context.Context, modelCode string, dto DefinitionDTO) (*DefinitionDTO, error) {
	kind, err := s.requireModelOperation(ctx, modelCode, dto.Kind, domain.CatalogOpUpdateDefinition)
	if err != nil {
		return nil, err
	}
	switch kind {
	case KindPersonality:
		return s.personality.updateDefinition(ctx, modelCode, dto)
	case KindBehaviorAbility:
		return s.updateBehaviorDefinition(ctx, modelCode, dto)
	default:
		return nil, invalidArgument("模型类型无效")
	}
}

func (s *service) Options(ctx context.Context, kind string) (*OptionsResult, error) {
	result := &OptionsResult{
		Kinds: apiKindOptions(),
		Algorithms: []Option{
			{Label: "MBTI", Value: "mbti"},
			{Label: "SBTI", Value: "sbti"},
			{Label: "Big Five", Value: "bigfive"},
			{Label: "自定义类型人格", Value: AlgorithmCustomTypology},
			{Label: "分数区间解释", Value: "score_range"},
		},
		SubKinds: []Option{
			{Label: "类型人格", Value: "typology"},
			{Label: "量表评分", Value: SubKindScale},
		},
	}
	if kind == "" || kind == KindBehaviorAbility {
		if s.behavior.cmd != nil {
			categories, err := s.behavior.cmd.Options(ctx)
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
	if kind, ok := s.resolveModelKind(ctx, modelCode); ok && kind == KindPersonality {
		return s.personality.validate(ctx, modelCode)
	}
	def, err := s.GetDefinition(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	var issues []ValidationIssue
	if len(def.Payload) == 0 {
		issues = append(issues, ValidationIssue{
			Field:   "definition.payload",
			Message: "模型定义 payload 不能为空",
			Code:    "definition.payload.required",
			Level:   "error",
		})
	}
	return NewValidationResult(issues), nil
}

func (s *service) PreviewReport(ctx context.Context, modelCode string, payload json.RawMessage) (*PreviewReportResult, error) {
	kind, err := s.requireModelOperationWithNotFound(ctx, modelCode, "", domain.CatalogOpPreview, modelNotFoundError())
	if err != nil {
		if errors.IsCode(err, code.ErrInvalidArgument) {
			return nil, errors.WithCode(code.ErrInvalidArgument, "预览报告生成尚未接入行为能力模型")
		}
		return nil, err
	}
	if kind != KindPersonality {
		return nil, errors.WithCode(code.ErrInvalidArgument, "预览报告生成尚未接入行为能力模型")
	}
	return s.personality.previewReport(ctx, modelCode, payload)
}

func (s *service) GetQRCode(ctx context.Context, modelCode string) (string, error) {
	if modelCode == "" {
		return "", invalidArgument("模型编码不能为空")
	}
	kind, err := s.requireModelOperationWithNotFound(ctx, modelCode, "", domain.CatalogOpQRCode, modelNotFoundError())
	if err != nil {
		return "", err
	}
	switch kind {
	case KindPersonality:
		return s.getPersonalityQRCode(ctx, modelCode)
	case KindBehaviorAbility:
		return s.behavior.getQRCode(ctx, modelCode)
	default:
		return "", invalidArgument("模型类型不支持二维码")
	}
}

func invalidArgument(format string, args ...interface{}) error {
	return errors.WithCode(code.ErrInvalidArgument, format, args...)
}

func unavailable(message string) error {
	return errors.WithCode(code.ErrInternalServerError, "%s", message)
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
