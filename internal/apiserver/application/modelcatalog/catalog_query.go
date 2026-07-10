package modelcatalog

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/hotrank"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// CatalogQueryDependencies are the read-side dependencies for the unified
// catalogue. The service deliberately receives model repositories rather than
// legacy scale read models or payload decoders.
type CatalogQueryDependencies struct {
	Models     modelcatalogport.ModelRepository
	Published  modelcatalogport.PublishedModelLister
	Authorizer Authorizer
	QRCode     qrcode.QRCodeService
	HotRank    hotrank.ReadModel
}

type catalogQueryService struct {
	deps CatalogQueryDependencies
}

func NewCatalogQueryService(deps CatalogQueryDependencies) CatalogQueryService {
	return &catalogQueryService{deps: deps}
}

func (s *catalogQueryService) Get(ctx context.Context, actor ActorContext, codeValue string) (*ModelSummary, error) {
	if err := s.authorize(ctx, actor, Resource{Code: codeValue}); err != nil {
		return nil, err
	}
	if codeValue == "" || s.deps.Models == nil {
		return nil, errors.WithCode(code.ErrInvalidArgument, "model code and repository are required")
	}
	model, err := s.deps.Models.FindByCode(ctx, codeValue)
	if err != nil {
		return nil, err
	}
	return modelSummaryFromAssessmentModel(model), nil
}

func (s *catalogQueryService) List(ctx context.Context, actor ActorContext, input ListModelsDTO) (*ModelListResult, error) {
	if err := s.authorize(ctx, actor, Resource{}); err != nil {
		return nil, err
	}
	if s.deps.Models == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "catalogue model repository is not configured")
	}
	filter, err := draftListFilter(input)
	if err != nil {
		return nil, err
	}
	models, total, err := s.deps.Models.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	result := &ModelListResult{Items: make([]ModelSummary, 0, len(models)), Total: total, Page: filter.Page, PageSize: filter.PageSize}
	for _, model := range models {
		result.Items = append(result.Items, *modelSummaryFromAssessmentModel(model))
	}
	return result, nil
}

func (s *catalogQueryService) GetPublished(ctx context.Context, actor ActorContext, codeValue, version string) (*PublishedModelDetail, error) {
	if err := s.authorize(ctx, actor, Resource{Code: codeValue}); err != nil {
		return nil, err
	}
	if codeValue == "" || s.deps.Published == nil {
		return nil, errors.WithCode(code.ErrInvalidArgument, "published model code and repository are required")
	}
	items, _, err := s.deps.Published.ListPublishedModels(ctx, modelcatalogport.ListPublishedFilter{
		Code:     codeValue,
		Page:     1,
		PageSize: 100,
	})
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Code == codeValue && (version == "" || item.Version == version) {
			return publishedDetailFromModel(item)
		}
	}
	return nil, domain.ErrNotFound
}

func (s *catalogQueryService) ListPublished(ctx context.Context, actor ActorContext, input ListModelsDTO) (*PublishedModelListResult, error) {
	if err := s.authorize(ctx, actor, Resource{}); err != nil {
		return nil, err
	}
	if s.deps.Published == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "published model repository is not configured")
	}
	filter, err := publishedListFilter(input)
	if err != nil {
		return nil, err
	}
	items, total, err := s.deps.Published.ListPublishedModels(ctx, filter)
	if err != nil {
		return nil, err
	}
	result := &PublishedModelListResult{Items: make([]PublishedModelDetail, 0, len(items)), Total: total, Page: filter.Page, PageSize: filter.PageSize}
	for _, item := range items {
		detail, err := publishedDetailFromModel(item)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, *detail)
	}
	return result, nil
}

func (s *catalogQueryService) ListHotPublished(ctx context.Context, actor ActorContext, input ListModelsDTO, limit, windowDays int) (*HotModelListResult, error) {
	if err := s.authorize(ctx, actor, Resource{}); err != nil {
		return nil, err
	}
	if s.deps.HotRank == nil {
		return &HotModelListResult{Items: []HotModelSummary{}, Limit: normalizeHotLimit(limit), WindowDays: normalizeHotWindow(windowDays)}, nil
	}
	if input.Kind != "" && input.Kind != KindScale {
		return &HotModelListResult{Items: []HotModelSummary{}, Limit: normalizeHotLimit(limit), WindowDays: normalizeHotWindow(windowDays)}, nil
	}
	entries, err := s.deps.HotRank.Top(ctx, hotrank.Query{Limit: normalizeHotLimit(limit), WindowDays: normalizeHotWindow(windowDays)})
	if err != nil {
		return nil, err
	}
	result := &HotModelListResult{Items: make([]HotModelSummary, 0, len(entries)), Limit: normalizeHotLimit(limit), WindowDays: normalizeHotWindow(windowDays)}
	for _, entry := range entries {
		published, err := s.ListPublished(ctx, actor, ListModelsDTO{Kind: string(domain.KindScale), QuestionnaireCode: entry.QuestionnaireCode, Page: 1, PageSize: 1})
		if err != nil {
			return nil, err
		}
		if len(published.Items) == 0 {
			continue
		}
		result.Items = append(result.Items, HotModelSummary{ModelSummary: published.Items[0].ModelSummary, Rank: len(result.Items) + 1, SubmissionCount: entry.Score, HeatScore: entry.Score})
	}
	result.Total = int64(len(result.Items))
	return result, nil
}

func (s *catalogQueryService) GetQuestionnaire(ctx context.Context, actor ActorContext, codeValue string) (*QuestionnaireBindingResult, error) {
	summary, err := s.Get(ctx, actor, codeValue)
	if err != nil {
		return nil, err
	}
	return &QuestionnaireBindingResult{QuestionnaireCode: summary.QuestionnaireCode, QuestionnaireVersion: summary.QuestionnaireVersion}, nil
}

func (s *catalogQueryService) Options(ctx context.Context, actor ActorContext, kind string) (*OptionsResult, error) {
	if err := s.authorize(ctx, actor, Resource{}); err != nil {
		return nil, err
	}
	if kind != "" && !IsSupportedAPIKind(kind) {
		return nil, errors.WithCode(code.ErrInvalidArgument, "model kind is invalid")
	}
	result := catalogOptionsForKind(kind)
	return &result, nil
}

func (s *catalogQueryService) GetQRCode(ctx context.Context, actor ActorContext, codeValue string) (string, error) {
	if err := s.authorize(ctx, actor, Resource{Code: codeValue}); err != nil {
		return "", err
	}
	if s.deps.Models == nil || s.deps.QRCode == nil {
		return "", errors.WithCode(code.ErrInternalServerError, "catalogue QR code service is not configured")
	}
	model, err := s.deps.Models.FindByCode(ctx, codeValue)
	if err != nil {
		return "", err
	}
	switch model.Kind {
	case domain.KindScale:
		return s.deps.QRCode.GenerateScaleQRCode(ctx, model.Code)
	case domain.KindTypology:
		return s.deps.QRCode.GeneratePersonalityAssessmentQRCode(ctx, model.Code)
	default:
		return "", errors.WithCode(code.ErrInvalidArgument, "model kind does not support QR code")
	}
}

func (s *catalogQueryService) authorize(ctx context.Context, actor ActorContext, resource Resource) error {
	if s.deps.Authorizer == nil {
		return errors.WithCode(code.ErrInternalServerError, "catalogue authorizer is not configured")
	}
	return s.deps.Authorizer.Authorize(ctx, actor, ActionReadCatalog, resource)
}

func draftListFilter(input ListModelsDTO) (modelcatalogport.ListFilter, error) {
	kind, err := kindFromListInput(input)
	if err != nil {
		return modelcatalogport.ListFilter{}, err
	}
	return modelcatalogport.ListFilter{Kind: kind, SubKind: domain.SubKind(input.SubKind), Status: domain.ModelStatus(input.Status), Keyword: input.Keyword, Category: input.Category, Algorithm: domain.Algorithm(input.Algorithm), QuestionnaireCode: input.QuestionnaireCode, QuestionnaireVersion: input.QuestionnaireVersion, Page: normalizePage(input.Page), PageSize: normalizePageSize(input.PageSize)}, nil
}

func publishedListFilter(input ListModelsDTO) (modelcatalogport.ListPublishedFilter, error) {
	kind, err := kindFromListInput(input)
	if err != nil {
		return modelcatalogport.ListPublishedFilter{}, err
	}
	return modelcatalogport.ListPublishedFilter{Kind: kind, SubKind: domain.SubKind(input.SubKind), Algorithm: domain.Algorithm(input.Algorithm), Category: input.Category, Keyword: input.Keyword, QuestionnaireCode: input.QuestionnaireCode, QuestionnaireVersion: input.QuestionnaireVersion, Page: normalizePage(input.Page), PageSize: normalizePageSize(input.PageSize)}, nil
}

func kindFromListInput(input ListModelsDTO) (domain.Kind, error) {
	if input.Kind == "" {
		return "", nil
	}
	kind, ok := APIKindToDomainKind(input.Kind)
	if !ok {
		return "", errors.WithCode(code.ErrInvalidArgument, "model kind is invalid")
	}
	return kind, nil
}

func publishedDetailFromModel(model *modelcatalogport.PublishedModel) (*PublishedModelDetail, error) {
	if model == nil {
		return nil, domain.ErrNotFound
	}
	if model.DefinitionV2 == nil {
		return nil, errors.WithCode(code.ErrInvalidArgument, "published model definition_v2 is required: %s", model.Code)
	}
	summary := ModelSummary{Code: model.Code, Kind: DomainKindToAPIKind(model.Kind), SubKind: string(model.SubKind), Algorithm: string(model.Algorithm), Title: model.Title, Description: model.Description, Status: model.Status, Category: model.Category, Stages: append([]string(nil), model.Stages...), ApplicableAges: append([]string(nil), model.ApplicableAges...), Reporters: append([]string(nil), model.Reporters...), Tags: append([]string(nil), model.Tags...), QuestionnaireCode: model.QuestionnaireCode, QuestionnaireVersion: model.QuestionnaireVersion}
	populateModelSummaryIdentity(&summary, model.Kind, model.SubKind, model.Algorithm, model.ProductChannel)
	return &PublishedModelDetail{ModelSummary: summary, Version: model.Version, Definition: model.DefinitionV2}, nil
}

func normalizePage(value int) int {
	if value > 0 {
		return value
	}
	return 1
}
func normalizePageSize(value int) int {
	if value > 0 && value <= 100 {
		return value
	}
	return 20
}
func normalizeHotLimit(value int) int {
	if value < 3 {
		return 5
	}
	if value > 5 {
		return 5
	}
	return value
}
func normalizeHotWindow(value int) int {
	if value <= 0 {
		return 30
	}
	return value
}

func scaleCategoryOptions() []Option {
	return []Option{{Value: "adhd", Label: "多动"}, {Value: "td", Label: "抽动"}, {Value: "asd", Label: "自闭"}, {Value: "pressure", Label: "压力"}, {Value: "sii", Label: "感觉统合"}, {Value: "efn", Label: "执行功能"}, {Value: "emt", Label: "情绪"}, {Value: "slp", Label: "睡眠"}, {Value: "personality", Label: "人格"}}
}

func scaleStageOptions() []Option {
	return []Option{{Value: "deep_assessment", Label: "深评"}, {Value: "follow_up", Label: "随访"}, {Value: "outcome", Label: "结局"}}
}

func scaleApplicableAgeOptions() []Option {
	return []Option{{Value: "infant", Label: "婴幼儿（0-3岁）"}, {Value: "preschool", Label: "学龄前（3-6岁）"}, {Value: "school_child", Label: "学龄儿童（6-12岁）"}, {Value: "adolescent", Label: "青少年（12-18岁）"}, {Value: "adult", Label: "成人（18岁以上）"}}
}

func scaleReporterOptions() []Option {
	return []Option{{Value: "parent", Label: "家长评"}, {Value: "teacher", Label: "教师评"}, {Value: "self", Label: "自评"}, {Value: "clinical", Label: "临床评定"}}
}

var _ CatalogQueryService = (*catalogQueryService)(nil)
