package modelcatalog

import (
	"context"
	"encoding/json"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavioral_rating"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/cognitive"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/personality"
	personalityconsumer "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/personality/consumer"
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
	BehavioralRatingCommand behavioral_rating.Service
	PersonalityCommand      personality.Service
	CognitiveCommand        cognitive.Service
	PersonalityQuery        personalityconsumer.PersonalityModelQueryService
	QuestionnaireQuery      questionnaireapp.QuestionnaireQueryService
	Codes                   codes.CodesService
	RawQRCodeGenerator      qrcode.QRCodeService
}

type service struct {
	deps             Dependencies
	behavioralRating behavioralRatingGateway
	personality      personalityGateway
	cognitive        cognitiveGateway
}

func NewService(deps Dependencies) Service {
	return &service{
		deps:             deps,
		behavioralRating: behavioralRatingGateway{cmd: deps.BehavioralRatingCommand},
		personality:      personalityGateway{cmd: deps.PersonalityCommand},
		cognitive:        cognitiveGateway{cmd: deps.CognitiveCommand},
	}
}

func (s *service) List(ctx context.Context, dto ListModelsDTO) (*ModelListResult, error) {
	if dto.Page <= 0 {
		dto.Page = 1
	}
	if dto.PageSize <= 0 {
		dto.PageSize = 20
	}
	if dto.Kind != "" && !domain.IsBehaviorAbilityProductChannelAPIKind(dto.Kind) {
		if err := requireCatalogOperation(dto.Kind, domain.CatalogOpList); err != nil {
			return nil, err
		}
	}

	result := &ModelListResult{Page: dto.Page, PageSize: dto.PageSize}
	if domain.IsBehaviorAbilityProductChannelAPIKind(dto.Kind) {
		return s.listBehaviorAbilityChannel(ctx, dto)
	}
	if shouldListModelKind(dto.Kind, KindPersonality) {
		items, total, err := s.listPersonality(ctx, dto)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, items...)
		result.Total += total
	}
	if shouldListModelKind(dto.Kind, KindCognitive) {
		items, err := s.listCognitive(ctx, dto)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, items.Items...)
		result.Total += items.Total
	}
	if shouldListModelKind(dto.Kind, KindBehavioralRating) {
		items, err := s.listBehavioralRating(ctx, dto)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, items.Items...)
		result.Total += items.Total
	}
	return result, nil
}

func (s *service) Create(ctx context.Context, dto CreateModelDTO) (*ModelSummary, error) {
	if dto.Kind == "" {
		return nil, invalidArgument("模型类型不能为空")
	}
	if domain.IsBehaviorAbilityProductChannelAPIKind(dto.Kind) {
		return nil, invalidArgument("模型类型无效")
	}
	if err := requireCatalogOperation(dto.Kind, domain.CatalogOpCreate); err != nil {
		return nil, err
	}
	switch dto.Kind {
	case KindPersonality:
		return s.personality.create(ctx, dto)
	case KindCognitive:
		return s.createCognitive(ctx, dto)
	case KindBehavioralRating:
		return s.createBehavioralRating(ctx, dto)
	default:
		return nil, invalidArgument("模型类型无效")
	}
}

func (s *service) Get(ctx context.Context, modelCode string) (*ModelSummary, error) {
	if modelCode == "" {
		return nil, invalidArgument("模型编码不能为空")
	}
	if s.personality.cmd != nil {
		if result, err := s.personality.cmd.Get(ctx, modelCode); err == nil && result != nil {
			return summaryFromPersonality(result), nil
		}
	}
	if s.cognitive.cmd != nil {
		if result, err := s.cognitive.cmd.Get(ctx, modelCode); err == nil && result != nil {
			return cognitiveSummaryFromResult(result), nil
		}
	}
	if s.behavioralRating.cmd != nil {
		if result, err := s.behavioralRating.cmd.Get(ctx, modelCode); err == nil && result != nil {
			return behavioralRatingSummaryFromResult(result), nil
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
	case KindCognitive:
		return s.cognitive.updateBasicInfo(ctx, dto)
	case KindBehavioralRating:
		return s.behavioralRating.updateBasicInfo(ctx, dto)
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
	case KindCognitive:
		return s.cognitive.delete(ctx, modelCode)
	case KindBehavioralRating:
		return s.behavioralRating.delete(ctx, modelCode)
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
	case KindCognitive:
		return s.cognitive.publish(ctx, modelCode)
	case KindBehavioralRating:
		return s.behavioralRating.publish(ctx, modelCode)
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
	case KindCognitive:
		return s.cognitive.unpublish(ctx, modelCode)
	case KindBehavioralRating:
		return s.behavioralRating.unpublish(ctx, modelCode)
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
	case KindCognitive:
		return s.cognitive.archive(ctx, modelCode)
	case KindBehavioralRating:
		return s.behavioralRating.archive(ctx, modelCode)
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
	case KindCognitive:
		return s.cognitive.bindQuestionnaire(ctx, dto)
	case KindBehavioralRating:
		return s.behavioralRating.bindQuestionnaire(ctx, dto)
	default:
		return nil, invalidArgument("模型类型无效")
	}
}

func (s *service) GetQuestionnaire(ctx context.Context, modelCode string) (*QuestionnaireBindingResult, error) {
	kind, ok := s.resolveModelKind(ctx, modelCode)
	if !ok {
		return nil, modelNotFoundError()
	}
	switch kind {
	case KindPersonality:
		return s.personality.getQuestionnaire(ctx, modelCode)
	case KindBehavioralRating:
		cmd, err := s.behavioralRating.require()
		if err != nil {
			return nil, err
		}
		model, err := cmd.Get(ctx, modelCode)
		if err != nil {
			return nil, err
		}
		return s.questionnaireBinding(ctx, model.QuestionnaireCode, model.QuestionnaireVersion)
	case KindCognitive:
		cmd, err := s.cognitive.require()
		if err != nil {
			return nil, err
		}
		model, err := cmd.Get(ctx, modelCode)
		if err != nil {
			return nil, err
		}
		return s.questionnaireBinding(ctx, model.QuestionnaireCode, model.QuestionnaireVersion)
	default:
		return nil, modelNotFoundError()
	}
}

func (s *service) GetDefinition(ctx context.Context, modelCode string) (*DefinitionDTO, error) {
	if kind, ok := s.resolveModelKind(ctx, modelCode); ok && kind == KindPersonality {
		return s.personality.getDefinition(ctx, modelCode)
	}
	if kind, ok := s.resolveModelKind(ctx, modelCode); ok && kind == KindCognitive {
		return s.cognitive.getDefinition(ctx, modelCode)
	}
	if kind, ok := s.resolveModelKind(ctx, modelCode); ok && kind == KindBehavioralRating {
		return s.behavioralRating.getDefinition(ctx, modelCode)
	}
	if s.deps.PersonalityQuery != nil {
		personality, err := s.deps.PersonalityQuery.GetPublishedByCode(ctx, modelCode)
		if err == nil && personality != nil {
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
	}
	return nil, modelNotFoundError()
}

func (s *service) UpdateDefinition(ctx context.Context, modelCode string, dto DefinitionDTO) (*DefinitionDTO, error) {
	kind, err := s.requireModelOperation(ctx, modelCode, dto.Kind, domain.CatalogOpUpdateDefinition)
	if err != nil {
		return nil, err
	}
	switch kind {
	case KindPersonality:
		return s.personality.updateDefinition(ctx, modelCode, dto)
	case KindCognitive:
		return s.cognitive.updateDefinition(ctx, modelCode, dto)
	case KindBehavioralRating:
		return s.behavioralRating.updateDefinition(ctx, modelCode, dto)
	default:
		return nil, invalidArgument("模型类型无效")
	}
}

func (s *service) Options(ctx context.Context, kind string) (*OptionsResult, error) {
	result := &OptionsResult{
		Kinds:             apiKindOptions(),
		ProductChannels:   productChannelOptions(),
		AlgorithmFamilies: algorithmFamilyOptions(),
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
	if kind == "" || domain.IsBehaviorAbilityProductChannelAPIKind(kind) {
		result.ModelFamilies = behaviorAbilityChannelModelFamilyOptions()
		result.Algorithms = append(result.Algorithms,
			Option{Label: "BRIEF-2", Value: string(domain.AlgorithmBrief2)},
			Option{Label: "SPM", Value: string(domain.AlgorithmSPM)},
		)
	}
	if kind == string(domain.KindBehavioralRating) {
		result.Algorithms = append(result.Algorithms, Option{Label: "BRIEF-2", Value: string(domain.AlgorithmBrief2)})
	}
	if kind == KindCognitive {
		result.Algorithms = append(result.Algorithms, Option{Label: "SPM", Value: string(domain.AlgorithmSPM)})
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
	if kind != KindPersonality {
		return "", invalidArgument("模型类型不支持二维码")
	}
	return s.getPersonalityQRCode(ctx, modelCode)
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
