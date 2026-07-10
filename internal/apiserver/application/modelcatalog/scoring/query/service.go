package query

import (
	"context"
	stderrors "errors"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/assessmentstore"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/ports"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel/hotrank"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

const (
	defaultHotScaleLimit      = 5
	minHotScaleLimit          = 3
	maxHotScaleLimit          = 5
	defaultHotScaleWindowDays = 30
	maxHotScaleWindowDays     = 365

	scaleStatusDraft    = "draft"
	scaleStatusArchived = "archived"
)

// 查询Service 量表查询服务实现
// 行为者：所有用户
type queryService struct {
	reader       scalereadmodel.ScaleReader
	identitySvc  iambridge.IdentityResolver
	listCache    scalelistcache.PublishedListCache
	hotListCache scalelistcache.HotListCache
	hotset       cachetarget.HotsetRecorder
	hotRank      hotrank.ReadModel
	modelRepo    modelcatalogport.ModelRepository
	published    modelcatalogport.PublishedModelRepository
	readerV2     modelcatalogport.PublishedModelReader
}

type ModelCatalogSources struct {
	ModelRepo       modelcatalogport.ModelRepository
	PublishedRepo   modelcatalogport.PublishedModelRepository
	PublishedReader modelcatalogport.PublishedModelReader
}

// NewQueryService 创建量表查询服务。
func NewQueryService(reader scalereadmodel.ScaleReader, identitySvc iambridge.IdentityResolver, listCache scalelistcache.PublishedListCache, hotset cachetarget.HotsetRecorder, hotRankReaders ...hotrank.ReadModel) ports.ScaleQueryService {
	return newQueryService(reader, identitySvc, listCache, nil, hotset, ModelCatalogSources{}, hotRankReaders...)
}

// NewQueryServiceWithReadModel 创建使用显式 read model 的量表查询服务。
func NewQueryServiceWithReadModel(reader scalereadmodel.ScaleReader, identitySvc iambridge.IdentityResolver, listCache scalelistcache.PublishedListCache, hotset cachetarget.HotsetRecorder, hotRankReaders ...hotrank.ReadModel) ports.ScaleQueryService {
	return newQueryService(reader, identitySvc, listCache, nil, hotset, ModelCatalogSources{}, hotRankReaders...)
}

// NewQueryServiceWithHotListCache 创建带热门量表列表缓存的查询服务。
func NewQueryServiceWithHotListCache(
	reader scalereadmodel.ScaleReader,
	identitySvc iambridge.IdentityResolver,
	listCache scalelistcache.PublishedListCache,
	hotListCache scalelistcache.HotListCache,
	hotset cachetarget.HotsetRecorder,
	hotRankReaders ...hotrank.ReadModel,
) ports.ScaleQueryService {
	return newQueryService(reader, identitySvc, listCache, hotListCache, hotset, ModelCatalogSources{}, hotRankReaders...)
}

func NewQueryServiceWithModelCatalogSources(
	reader scalereadmodel.ScaleReader,
	identitySvc iambridge.IdentityResolver,
	listCache scalelistcache.PublishedListCache,
	hotListCache scalelistcache.HotListCache,
	hotset cachetarget.HotsetRecorder,
	sources ModelCatalogSources,
	hotRankReaders ...hotrank.ReadModel,
) ports.ScaleQueryService {
	return newQueryService(reader, identitySvc, listCache, hotListCache, hotset, sources, hotRankReaders...)
}

func newQueryService(
	reader scalereadmodel.ScaleReader,
	identitySvc iambridge.IdentityResolver,
	listCache scalelistcache.PublishedListCache,
	hotListCache scalelistcache.HotListCache,
	hotset cachetarget.HotsetRecorder,
	sources ModelCatalogSources,
	hotRankReaders ...hotrank.ReadModel,
) ports.ScaleQueryService {
	var hotRank hotrank.ReadModel
	if len(hotRankReaders) > 0 {
		hotRank = hotRankReaders[0]
	}
	return &queryService{
		reader:       reader,
		identitySvc:  identitySvc,
		listCache:    listCache,
		hotListCache: hotListCache,
		hotset:       hotset,
		hotRank:      hotRank,
		modelRepo:    sources.ModelRepo,
		published:    sources.PublishedRepo,
		readerV2:     sources.PublishedReader,
	}
}

// GetByCode 根据编码获取量表
func (s *queryService) GetByCode(ctx context.Context, code string) (*shared.ScaleResult, error) {
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	result, err := s.getScaleResultFromAssessmentModel(ctx, code)
	if err != nil {
		return nil, err
	}
	s.recordHotset(ctx, cachetarget.NewStaticScaleWarmupTarget(code))
	return result, nil
}

// GetByQuestionnaireCode 根据问卷编码获取量表
func (s *queryService) GetByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*shared.ScaleResult, error) {
	if questionnaireCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "问卷编码不能为空")
	}
	if s == nil || s.modelRepo == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "测评模型仓储未配置")
	}
	model, err := assessmentstore.FindScaleByQuestionnaireCode(ctx, s.modelRepo, questionnaireCode)
	if err != nil {
		return nil, err
	}
	result, err := legacyadapter.ScaleResultFromAssessmentModel(model)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "解析量表模型失败")
	}
	return result, nil
}

// List 查询量表摘要列表
func (s *queryService) List(ctx context.Context, dto shared.ListScalesDTO) (*shared.ScaleSummaryListResult, error) {
	if err := validateScaleListPage(dto.Page, dto.PageSize); err != nil {
		return nil, err
	}
	filter, err := s.normalizeScaleFilter(dto.Filter)
	if err != nil {
		return nil, err
	}

	return s.listScaleSummaryRows(ctx, filter, dto.Page, dto.PageSize)
}

// GetPublishedByCode 获取已发布的量表
func (s *queryService) GetPublishedByCode(ctx context.Context, code string) (*shared.ScaleResult, error) {
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	result, err := s.getScaleResultFromPublishedModel(ctx, code)
	if err != nil {
		return nil, err
	}
	s.recordHotset(ctx, cachetarget.NewStaticScaleWarmupTarget(code))
	return result, nil
}

// GetFactors 获取量表的因子列表
func (s *queryService) GetFactors(ctx context.Context, scaleCode string) ([]shared.FactorResult, error) {
	if scaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	result, err := s.getScaleResultFromAssessmentModel(ctx, scaleCode)
	if err != nil {
		return nil, err
	}
	return append([]shared.FactorResult(nil), result.Factors...), nil
}

// ResolveAssessmentScaleContext 按问卷编码和版本解析创建测评所需的量表上下文。
func (s *queryService) ResolveAssessmentScaleContext(ctx context.Context, questionnaireCode, questionnaireVersion string) (*shared.AssessmentScaleContextResult, error) {
	if s == nil || questionnaireCode == "" {
		return &shared.AssessmentScaleContextResult{}, nil
	}
	if questionnaireVersion != "" {
		if result, ok, err := s.assessmentScaleContextFromPublishedModel(ctx, questionnaireCode, questionnaireVersion); err != nil {
			return nil, err
		} else if ok {
			return result, nil
		}
	}
	if s.readerV2 == nil {
		return &shared.AssessmentScaleContextResult{}, nil
	}
	snapshot, err := s.readerV2.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		if domain.IsNotFound(err) || stderrors.Is(err, domain.ErrAmbiguousVersion) {
			return &shared.AssessmentScaleContextResult{}, nil
		}
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取已发布测评模型失败")
	}
	if snapshot == nil || snapshot.Kind != domain.KindScale {
		return &shared.AssessmentScaleContextResult{}, nil
	}
	return publishedScaleContext(snapshot), nil
}

func (s *queryService) getScaleResultFromAssessmentModel(ctx context.Context, code string) (*shared.ScaleResult, error) {
	if s == nil || s.modelRepo == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "测评模型仓储未配置")
	}
	model, err := s.modelRepo.FindByCode(ctx, code)
	if err != nil {
		if domain.IsNotFound(err) {
			return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
		}
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取测评模型失败")
	}
	if model == nil || model.Kind != domain.KindScale {
		return nil, errors.WithCode(errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}
	result, err := legacyadapter.ScaleResultFromAssessmentModel(model)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "解析量表模型失败")
	}
	return result, nil
}

func (s *queryService) getScaleResultFromPublishedModel(ctx context.Context, code string) (*shared.ScaleResult, error) {
	if s == nil || s.published == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "已发布测评模型仓储未配置")
	}
	snapshot, err := s.published.FindLatestPublishedByModelCode(ctx, domain.KindScale, code)
	if err != nil {
		if domain.IsNotFound(err) {
			return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
		}
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取已发布测评模型失败")
	}
	result, err := legacyadapter.ScaleResultFromPublishedModel(snapshot)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "解析已发布量表模型失败")
	}
	return result, nil
}

func (s *queryService) assessmentScaleContextFromPublishedModel(ctx context.Context, questionnaireCode, questionnaireVersion string) (*shared.AssessmentScaleContextResult, bool, error) {
	if s == nil || s.readerV2 == nil {
		return nil, false, nil
	}
	snapshot, err := s.readerV2.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		if domain.IsNotFound(err) || stderrors.Is(err, domain.ErrAmbiguousVersion) {
			return nil, false, nil
		}
		return nil, false, errors.WrapC(err, errorCode.ErrDatabase, "获取已发布测评模型失败")
	}
	if snapshot == nil || snapshot.Kind != domain.KindScale {
		return nil, false, nil
	}
	return publishedScaleContext(snapshot), true, nil
}

func publishedScaleContext(snapshot *modelcatalogport.PublishedModel) *shared.AssessmentScaleContextResult {
	scaleVersion := snapshot.Version
	if result, err := legacyadapter.ScaleResultFromPublishedModel(snapshot); err == nil && result != nil && result.ScaleVersion != "" {
		scaleVersion = result.ScaleVersion
	}
	scaleCode := snapshot.Code
	scaleName := snapshot.Title
	return &shared.AssessmentScaleContextResult{
		MedicalScaleCode: &scaleCode,
		MedicalScaleName: &scaleName,
		ScaleVersion:     &scaleVersion,
	}
}

func (s *queryService) recordHotset(ctx context.Context, target cachetarget.WarmupTarget) {
	if s == nil || s.hotset == nil {
		return
	}
	_ = s.hotset.Record(ctx, target)
}

func (s *queryService) normalizeScaleFilter(filter shared.ScaleListFilter) (scalereadmodel.ScaleFilter, error) {
	normalized := scalereadmodel.ScaleFilter{
		Status:   filter.Status,
		Title:    filter.Title,
		Category: filter.Category,
	}
	if normalized.Status != "" {
		parsed, ok := normalizeScaleStatus(normalized.Status)
		if !ok {
			return scalereadmodel.ScaleFilter{}, errors.WithCode(errorCode.ErrInvalidArgument, "状态无效")
		}
		normalized.Status = parsed
	}
	return normalized, nil
}

func normalizeScaleStatus(value string) (string, bool) {
	switch value {
	case scaleStatusDraft, scalereadmodel.ScaleStatusPublished, scaleStatusArchived:
		return value, true
	default:
		return "", false
	}
}
