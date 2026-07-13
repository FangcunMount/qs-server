// Package query owns catalog read use cases and their transport-neutral projections.
package query

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/hotrank"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Dependencies are the read-side dependencies for the unified catalogue. The
// service deliberately receives model repositories rather than legacy scale
// read models or payload decoders.
type Dependencies struct {
	Models     modelcatalogport.ModelRepository
	Published  modelcatalogport.PublishedModelLister
	Authorizer modelcatalog.Authorizer
	QRCode     qrcode.QRCodeService
	HotRank    hotrank.ReadModel
}

type catalogQueryService struct {
	deps Dependencies
}

func NewService(deps Dependencies) modelcatalog.CatalogQueryService {
	return &catalogQueryService{deps: deps}
}

func (s *catalogQueryService) Get(ctx context.Context, actor modelcatalog.ActorContext, codeValue string) (*modelcatalog.ModelSummary, error) {
	if err := s.authorize(ctx, actor, modelcatalog.Resource{Code: codeValue}); err != nil {
		return nil, err
	}
	if codeValue == "" || s.deps.Models == nil {
		return nil, errors.WithCode(code.ErrInvalidArgument, "model code and repository are required")
	}
	model, err := s.deps.Models.FindByCode(ctx, codeValue)
	if err != nil {
		return nil, err
	}
	return modelcatalog.ModelSummaryFromAssessmentModel(model), nil
}

func (s *catalogQueryService) List(ctx context.Context, actor modelcatalog.ActorContext, input modelcatalog.ListModelsDTO) (*modelcatalog.ModelListResult, error) {
	if err := s.authorize(ctx, actor, modelcatalog.Resource{}); err != nil {
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
	result := &modelcatalog.ModelListResult{Items: make([]modelcatalog.ModelSummary, 0, len(models)), Total: total, Page: filter.Page, PageSize: filter.PageSize}
	for _, model := range models {
		result.Items = append(result.Items, *modelcatalog.ModelSummaryFromAssessmentModel(model))
	}
	return result, nil
}

func (s *catalogQueryService) GetPublished(ctx context.Context, actor modelcatalog.ActorContext, codeValue, version string) (*modelcatalog.PublishedModelDetail, error) {
	if err := s.authorizePublished(ctx, actor, modelcatalog.Resource{Code: codeValue}); err != nil {
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
	logger.L(ctx).Warnw("published assessment model was not found in catalog lookup",
		"action", "get_published_assessment_model",
		"model_code", codeValue,
		"requested_version", version,
		"returned_item_count", len(items),
	)
	return nil, domain.ErrNotFound
}

func (s *catalogQueryService) ListPublished(ctx context.Context, actor modelcatalog.ActorContext, input modelcatalog.ListModelsDTO) (*modelcatalog.PublishedModelListResult, error) {
	if err := s.authorizePublished(ctx, actor, modelcatalog.Resource{}); err != nil {
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
	result := &modelcatalog.PublishedModelListResult{Items: make([]modelcatalog.PublishedModelDetail, 0, len(items)), Total: total, Page: filter.Page, PageSize: filter.PageSize}
	for _, item := range items {
		detail, err := publishedDetailFromModel(item)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, *detail)
	}
	return result, nil
}

func (s *catalogQueryService) ListHotPublished(ctx context.Context, actor modelcatalog.ActorContext, input modelcatalog.ListModelsDTO, limit, windowDays int) (*modelcatalog.HotModelListResult, error) {
	if err := s.authorizePublished(ctx, actor, modelcatalog.Resource{}); err != nil {
		return nil, err
	}
	if s.deps.HotRank == nil {
		return &modelcatalog.HotModelListResult{Items: []modelcatalog.HotModelSummary{}, Limit: normalizeHotLimit(limit), WindowDays: normalizeHotWindow(windowDays)}, nil
	}
	if input.Kind != "" && input.Kind != modelcatalog.KindScale {
		return &modelcatalog.HotModelListResult{Items: []modelcatalog.HotModelSummary{}, Limit: normalizeHotLimit(limit), WindowDays: normalizeHotWindow(windowDays)}, nil
	}
	entries, err := s.deps.HotRank.Top(ctx, hotrank.Query{Limit: normalizeHotLimit(limit), WindowDays: normalizeHotWindow(windowDays)})
	if err != nil {
		return nil, err
	}
	result := &modelcatalog.HotModelListResult{Items: make([]modelcatalog.HotModelSummary, 0, len(entries)), Limit: normalizeHotLimit(limit), WindowDays: normalizeHotWindow(windowDays)}
	for _, entry := range entries {
		published, err := s.ListPublished(ctx, actor, modelcatalog.ListModelsDTO{Kind: string(domain.KindScale), QuestionnaireCode: entry.QuestionnaireCode, Page: 1, PageSize: 1})
		if err != nil {
			return nil, err
		}
		if len(published.Items) == 0 {
			continue
		}
		result.Items = append(result.Items, modelcatalog.HotModelSummary{ModelSummary: published.Items[0].ModelSummary, Rank: len(result.Items) + 1, SubmissionCount: entry.Score, HeatScore: entry.Score})
	}
	result.Total = int64(len(result.Items))
	return result, nil
}

func (s *catalogQueryService) GetQuestionnaire(ctx context.Context, actor modelcatalog.ActorContext, codeValue string) (*modelcatalog.QuestionnaireBindingResult, error) {
	summary, err := s.Get(ctx, actor, codeValue)
	if err != nil {
		return nil, err
	}
	return &modelcatalog.QuestionnaireBindingResult{QuestionnaireCode: summary.QuestionnaireCode, QuestionnaireVersion: summary.QuestionnaireVersion}, nil
}

func (s *catalogQueryService) Options(ctx context.Context, actor modelcatalog.ActorContext, kind string) (*modelcatalog.OptionsResult, error) {
	if err := s.authorizePublished(ctx, actor, modelcatalog.Resource{}); err != nil {
		return nil, err
	}
	if kind != "" && !modelcatalog.IsSupportedAPIKind(kind) {
		return nil, errors.WithCode(code.ErrInvalidArgument, "model kind is invalid")
	}
	result := catalogOptionsForKind(kind)
	return &result, nil
}

func (s *catalogQueryService) GetQRCode(ctx context.Context, actor modelcatalog.ActorContext, codeValue string) (string, error) {
	if err := s.authorize(ctx, actor, modelcatalog.Resource{Code: codeValue}); err != nil {
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

func (s *catalogQueryService) authorize(ctx context.Context, actor modelcatalog.ActorContext, resource modelcatalog.Resource) error {
	if s.deps.Authorizer == nil {
		return errors.WithCode(code.ErrInternalServerError, "catalogue authorizer is not configured")
	}
	return s.deps.Authorizer.Authorize(ctx, actor, modelcatalog.ActionReadCatalog, resource)
}

// authorizePublished allows trusted service actors (gRPC catalogue) to resolve
// published models without an IAM user snapshot, while operator HTTP reads keep
// ActionReadCatalog + snapshot capability checks.
func (s *catalogQueryService) authorizePublished(ctx context.Context, actor modelcatalog.ActorContext, resource modelcatalog.Resource) error {
	if s.deps.Authorizer == nil {
		return errors.WithCode(code.ErrInternalServerError, "catalogue authorizer is not configured")
	}
	action := modelcatalog.ActionReadCatalog
	if modelcatalog.IsTrustedServiceActor(actor) {
		action = modelcatalog.ActionResolvePublished
	}
	return s.deps.Authorizer.Authorize(ctx, actor, action, resource)
}

func draftListFilter(input modelcatalog.ListModelsDTO) (modelcatalogport.ListFilter, error) {
	kind, err := kindFromListInput(input)
	if err != nil {
		return modelcatalogport.ListFilter{}, err
	}
	return modelcatalogport.ListFilter{Kind: kind, SubKind: domain.SubKind(input.SubKind), Status: domain.ModelStatus(input.Status), Keyword: input.Keyword, Category: input.Category, Algorithm: domain.Algorithm(input.Algorithm), ProductChannel: domain.ProductChannel(input.ProductChannel), QuestionnaireCode: input.QuestionnaireCode, QuestionnaireVersion: input.QuestionnaireVersion, Page: normalizePage(input.Page), PageSize: normalizePageSize(input.PageSize)}, nil
}

func publishedListFilter(input modelcatalog.ListModelsDTO) (modelcatalogport.ListPublishedFilter, error) {
	kind, err := kindFromListInput(input)
	if err != nil {
		return modelcatalogport.ListPublishedFilter{}, err
	}
	return modelcatalogport.ListPublishedFilter{Kind: kind, SubKind: domain.SubKind(input.SubKind), Algorithm: domain.Algorithm(input.Algorithm), ProductChannel: domain.ProductChannel(input.ProductChannel), Category: input.Category, Keyword: input.Keyword, QuestionnaireCode: input.QuestionnaireCode, QuestionnaireVersion: input.QuestionnaireVersion, Page: normalizePage(input.Page), PageSize: normalizePageSize(input.PageSize)}, nil
}

func kindFromListInput(input modelcatalog.ListModelsDTO) (domain.Kind, error) {
	if input.Kind == "" {
		return "", nil
	}
	kind, ok := modelcatalog.APIKindToDomainKind(input.Kind)
	if !ok {
		return "", errors.WithCode(code.ErrInvalidArgument, "model kind is invalid")
	}
	return kind, nil
}

func publishedDetailFromModel(model *modelcatalogport.PublishedModel) (*modelcatalog.PublishedModelDetail, error) {
	if model == nil {
		return nil, domain.ErrNotFound
	}
	if model.DefinitionV2 == nil {
		return nil, errors.WithCode(code.ErrInvalidArgument, "published model definition_v2 is required: %s", model.Code)
	}
	summary := modelcatalog.ModelSummary{Code: model.Code, Kind: modelcatalog.DomainKindToAPIKind(model.Kind), SubKind: string(model.SubKind), Algorithm: string(model.Algorithm), Title: model.Title, Description: model.Description, Status: model.Status, Category: model.Category, Stages: append([]string(nil), model.Stages...), ApplicableAges: append([]string(nil), model.ApplicableAges...), Reporters: append([]string(nil), model.Reporters...), Tags: append([]string(nil), model.Tags...), QuestionnaireCode: model.QuestionnaireCode, QuestionnaireVersion: model.QuestionnaireVersion}
	modelcatalog.PopulateModelSummaryIdentity(&summary, model.Kind, model.SubKind, model.Algorithm, model.ProductChannel)
	return &modelcatalog.PublishedModelDetail{ModelSummary: summary, Version: model.Version, Definition: model.DefinitionV2}, nil
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

func scaleCategoryOptions() []modelcatalog.Option {
	return []modelcatalog.Option{{Value: "adhd", Label: "多动"}, {Value: "td", Label: "抽动"}, {Value: "asd", Label: "自闭"}, {Value: "pressure", Label: "压力"}, {Value: "sii", Label: "感觉统合"}, {Value: "efn", Label: "执行功能"}, {Value: "emt", Label: "情绪"}, {Value: "slp", Label: "睡眠"}, {Value: "personality", Label: "人格"}}
}

func scaleStageOptions() []modelcatalog.Option {
	return []modelcatalog.Option{{Value: "deep_assessment", Label: "深评"}, {Value: "follow_up", Label: "随访"}, {Value: "outcome", Label: "结局"}}
}

func scaleApplicableAgeOptions() []modelcatalog.Option {
	return []modelcatalog.Option{{Value: "infant", Label: "婴幼儿（0-3岁）"}, {Value: "preschool", Label: "学龄前（3-6岁）"}, {Value: "school_child", Label: "学龄儿童（6-12岁）"}, {Value: "adolescent", Label: "青少年（12-18岁）"}, {Value: "adult", Label: "成人（18岁以上）"}}
}

func scaleReporterOptions() []modelcatalog.Option {
	return []modelcatalog.Option{{Value: "parent", Label: "家长评"}, {Value: "teacher", Label: "教师评"}, {Value: "self", Label: "自评"}, {Value: "clinical", Label: "临床评定"}}
}

var _ modelcatalog.CatalogQueryService = (*catalogQueryService)(nil)
