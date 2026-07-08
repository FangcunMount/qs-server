package modelcatalog

import (
	"context"
	"encoding/json"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/norming"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/taskperformance"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/typology"
	typologyconsumer "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/typology/consumer"
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
	NormingCommand         norming.Service
	TypologyCommand        typology.Service
	TaskPerformanceCommand taskperformance.Service
	TypologyQuery          typologyconsumer.TypologyModelQueryService
	QuestionnaireQuery     questionnaireapp.QuestionnaireQueryService
	Codes                  codes.CodesService
	RawQRCodeGenerator     qrcode.QRCodeService
}

type service struct {
	deps                Dependencies
	normingKind         normingKindGateway
	typologyKind        typologyKindGateway
	taskPerformanceKind taskPerformanceKindGateway
}

func NewService(deps Dependencies) Service {
	return &service{
		deps:                deps,
		normingKind:         normingKindGateway{cmd: deps.NormingCommand},
		typologyKind:        typologyKindGateway{cmd: deps.TypologyCommand},
		taskPerformanceKind: taskPerformanceKindGateway{cmd: deps.TaskPerformanceCommand},
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
		if domain.IsBehaviorAbilityProductChannelAPIKind(dto.Kind) {
			return nil, invalidArgument("behavior_ability 产品通道不再支持 List 聚合，请分别调用 behavioral_rating 或 cognitive")
		}
		if err := requireCatalogOperation(dto.Kind, domain.CatalogOpList); err != nil {
			return nil, err
		}
	}

	result := &ModelListResult{Page: dto.Page, PageSize: dto.PageSize}
	if shouldListModelKind(dto.Kind, KindPersonality) {
		items, total, err := s.listTypology(ctx, dto)
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
		items, err := s.listNormingModels(ctx, dto)
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
		return s.typologyKind.create(ctx, dto)
	case KindCognitive:
		return s.createCognitive(ctx, dto)
	case KindBehavioralRating:
		return s.createNormingModel(ctx, dto)
	default:
		return nil, invalidArgument("模型类型无效")
	}
}

func (s *service) Get(ctx context.Context, modelCode string) (*ModelSummary, error) {
	if modelCode == "" {
		return nil, invalidArgument("模型编码不能为空")
	}
	if s.typologyKind.cmd != nil {
		if result, err := s.typologyKind.cmd.Get(ctx, modelCode); err == nil && result != nil {
			return summaryFromTypology(result), nil
		}
	}
	if s.taskPerformanceKind.cmd != nil {
		if result, err := s.taskPerformanceKind.cmd.Get(ctx, modelCode); err == nil && result != nil {
			return cognitiveSummaryFromResult(result), nil
		}
	}
	if s.normingKind.cmd != nil {
		if result, err := s.normingKind.cmd.Get(ctx, modelCode); err == nil && result != nil {
			return normingSummaryFromResult(result), nil
		}
	}
	if s.deps.TypologyQuery != nil {
		if result, err := s.deps.TypologyQuery.GetPublishedByCode(ctx, modelCode); err == nil && result != nil {
			return typologySummaryFromDetail(result), nil
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
		return s.typologyKind.updateBasicInfo(ctx, dto)
	case KindCognitive:
		return s.taskPerformanceKind.updateBasicInfo(ctx, dto)
	case KindBehavioralRating:
		return s.normingKind.updateBasicInfo(ctx, dto)
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
		return s.typologyKind.delete(ctx, modelCode)
	case KindCognitive:
		return s.taskPerformanceKind.delete(ctx, modelCode)
	case KindBehavioralRating:
		return s.normingKind.delete(ctx, modelCode)
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
		return s.typologyKind.publish(ctx, modelCode)
	case KindCognitive:
		return s.taskPerformanceKind.publish(ctx, modelCode)
	case KindBehavioralRating:
		return s.normingKind.publish(ctx, modelCode)
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
		return s.typologyKind.unpublish(ctx, modelCode)
	case KindCognitive:
		return s.taskPerformanceKind.unpublish(ctx, modelCode)
	case KindBehavioralRating:
		return s.normingKind.unpublish(ctx, modelCode)
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
		return s.typologyKind.archive(ctx, modelCode)
	case KindCognitive:
		return s.taskPerformanceKind.archive(ctx, modelCode)
	case KindBehavioralRating:
		return s.normingKind.archive(ctx, modelCode)
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
		return s.typologyKind.bindQuestionnaire(ctx, dto)
	case KindCognitive:
		return s.taskPerformanceKind.bindQuestionnaire(ctx, dto)
	case KindBehavioralRating:
		return s.normingKind.bindQuestionnaire(ctx, dto)
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
		return s.typologyKind.getQuestionnaire(ctx, modelCode)
	case KindBehavioralRating:
		cmd, err := s.normingKind.require()
		if err != nil {
			return nil, err
		}
		model, err := cmd.Get(ctx, modelCode)
		if err != nil {
			return nil, err
		}
		return s.questionnaireBinding(ctx, model.QuestionnaireCode, model.QuestionnaireVersion)
	case KindCognitive:
		cmd, err := s.taskPerformanceKind.require()
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
		return s.typologyKind.getDefinition(ctx, modelCode)
	}
	if kind, ok := s.resolveModelKind(ctx, modelCode); ok && kind == KindCognitive {
		return s.taskPerformanceKind.getDefinition(ctx, modelCode)
	}
	if kind, ok := s.resolveModelKind(ctx, modelCode); ok && kind == KindBehavioralRating {
		return s.normingKind.getDefinition(ctx, modelCode)
	}
	if s.deps.TypologyQuery != nil {
		personality, err := s.deps.TypologyQuery.GetPublishedByCode(ctx, modelCode)
		if err == nil && personality != nil {
			payload, marshalErr := json.Marshal(newTypologyDefinitionPayload(personality))
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
		return s.typologyKind.updateDefinition(ctx, modelCode, dto)
	case KindCognitive:
		return s.taskPerformanceKind.updateDefinition(ctx, modelCode, dto)
	case KindBehavioralRating:
		return s.normingKind.updateDefinition(ctx, modelCode, dto)
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
		result.ModelFamilies = behaviorAbilityProductChannelModelFamilyOptions()
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
		return s.typologyKind.validate(ctx, modelCode)
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
	return s.typologyKind.previewReport(ctx, modelCode, payload)
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
	return s.getTypologyQRCode(ctx, modelCode)
}

func behaviorAbilityProductChannelModelFamilyOptions() []Option {
	return []Option{
		{Label: "行为评定", Value: string(domain.KindBehavioralRating)},
		{Label: "认知能力", Value: string(domain.KindCognitive)},
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
