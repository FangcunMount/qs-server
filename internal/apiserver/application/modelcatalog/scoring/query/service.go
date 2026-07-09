package query

import (
	"context"
	stderrors "errors"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/ports"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition/hotrank"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

const (
	defaultHotScaleLimit      = 5
	minHotScaleLimit          = 3
	maxHotScaleLimit          = 5
	defaultHotScaleWindowDays = 30
	maxHotScaleWindowDays     = 365
)

// 查询Service 量表查询服务实现
// 行为者：所有用户
type queryService struct {
	repo         scaleQueryRepository
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

type scaleQueryRepository interface {
	FindByCode(ctx context.Context, code string) (*scaledefinition.MedicalScale, error)
	FindPublishedByCode(ctx context.Context, code string) (*scaledefinition.MedicalScale, error)
	FindByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*scaledefinition.MedicalScale, error)
	FindPublishedByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*scaledefinition.MedicalScale, error)
	FindByQuestionnaireRef(ctx context.Context, questionnaireCode, questionnaireVersion string) (*scaledefinition.MedicalScale, error)
}

// NewQueryService 创建量表查询服务。
func NewQueryService(repo scaleQueryRepository, reader scalereadmodel.ScaleReader, identitySvc iambridge.IdentityResolver, listCache scalelistcache.PublishedListCache, hotset cachetarget.HotsetRecorder, hotRankReaders ...hotrank.ReadModel) ports.ScaleQueryService {
	return newQueryService(repo, reader, identitySvc, listCache, nil, hotset, hotRankReaders...)
}

// NewQueryServiceWithReadModel 创建使用显式 read model 的量表查询服务。
func NewQueryServiceWithReadModel(repo scaleQueryRepository, reader scalereadmodel.ScaleReader, identitySvc iambridge.IdentityResolver, listCache scalelistcache.PublishedListCache, hotset cachetarget.HotsetRecorder, hotRankReaders ...hotrank.ReadModel) ports.ScaleQueryService {
	return newQueryService(repo, reader, identitySvc, listCache, nil, hotset, hotRankReaders...)
}

// NewQueryServiceWithHotListCache 创建带热门量表列表缓存的查询服务。
func NewQueryServiceWithHotListCache(
	repo scaleQueryRepository,
	reader scalereadmodel.ScaleReader,
	identitySvc iambridge.IdentityResolver,
	listCache scalelistcache.PublishedListCache,
	hotListCache scalelistcache.HotListCache,
	hotset cachetarget.HotsetRecorder,
	hotRankReaders ...hotrank.ReadModel,
) ports.ScaleQueryService {
	return newQueryService(repo, reader, identitySvc, listCache, hotListCache, hotset, hotRankReaders...)
}

func NewQueryServiceWithModelCatalogSources(
	repo scaleQueryRepository,
	reader scalereadmodel.ScaleReader,
	identitySvc iambridge.IdentityResolver,
	listCache scalelistcache.PublishedListCache,
	hotListCache scalelistcache.HotListCache,
	hotset cachetarget.HotsetRecorder,
	sources ModelCatalogSources,
	hotRankReaders ...hotrank.ReadModel,
) ports.ScaleQueryService {
	service := newQueryService(repo, reader, identitySvc, listCache, hotListCache, hotset, hotRankReaders...)
	if query, ok := service.(*queryService); ok {
		query.modelRepo = sources.ModelRepo
		query.published = sources.PublishedRepo
		query.readerV2 = sources.PublishedReader
	}
	return service
}

func newQueryService(
	repo scaleQueryRepository,
	reader scalereadmodel.ScaleReader,
	identitySvc iambridge.IdentityResolver,
	listCache scalelistcache.PublishedListCache,
	hotListCache scalelistcache.HotListCache,
	hotset cachetarget.HotsetRecorder,
	hotRankReaders ...hotrank.ReadModel,
) ports.ScaleQueryService {
	var hotRank hotrank.ReadModel
	if len(hotRankReaders) > 0 {
		hotRank = hotRankReaders[0]
	}
	return &queryService{
		repo:         repo,
		reader:       reader,
		identitySvc:  identitySvc,
		listCache:    listCache,
		hotListCache: hotListCache,
		hotset:       hotset,
		hotRank:      hotRank,
	}
}

// GetByCode 根据编码获取量表
func (s *queryService) GetByCode(ctx context.Context, code string) (*shared.ScaleResult, error) {
	// 1. 验证输入参数
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	// 2. 从仓储获取量表
	if result, ok, err := s.getScaleResultFromAssessmentModel(ctx, code); err != nil {
		return nil, err
	} else if ok {
		s.recordHotset(ctx, cachetarget.NewStaticScaleWarmupTarget(code))
		return result, nil
	}
	// TODO: remove legacy scales fallback after scale query migration completes.
	m, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}
	s.recordHotset(ctx, cachetarget.NewStaticScaleWarmupTarget(code))

	return shared.ToScaleResultWithUsers(ctx, m, s.identitySvc), nil
}

// GetByQuestionnaireCode 根据问卷编码获取量表
func (s *queryService) GetByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*shared.ScaleResult, error) {
	// 1. 验证输入参数
	if questionnaireCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "问卷编码不能为空")
	}

	// 2. 从仓储获取量表
	// TODO: remove legacy scales fallback after scale query migration supports questionnaire-code draft lookup.
	m, err := s.repo.FindByQuestionnaireCode(ctx, questionnaireCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	return shared.ToScaleResultWithUsers(ctx, m, s.identitySvc), nil
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
	// 1. 验证输入参数
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	// 2. 获取量表
	if result, ok, err := s.getScaleResultFromPublishedModel(ctx, code); err != nil {
		return nil, err
	} else if ok {
		s.recordHotset(ctx, cachetarget.NewStaticScaleWarmupTarget(code))
		return result, nil
	}
	// TODO: remove legacy scales fallback after published scale queries fully read published_assessment_models.
	m, err := s.repo.FindPublishedByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}
	s.recordHotset(ctx, cachetarget.NewStaticScaleWarmupTarget(code))

	return shared.ToScaleResultWithUsers(ctx, m, s.identitySvc), nil
}

// GetFactors 获取量表的因子列表
func (s *queryService) GetFactors(ctx context.Context, scaleCode string) ([]shared.FactorResult, error) {
	// 1. 验证输入参数
	if scaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	// 2. 从仓储获取量表
	if result, ok, err := s.getScaleResultFromAssessmentModel(ctx, scaleCode); err != nil {
		return nil, err
	} else if ok {
		return append([]shared.FactorResult(nil), result.Factors...), nil
	}
	// TODO: remove legacy scales fallback after factor queries fully read assessment_models.
	m, err := s.repo.FindByCode(ctx, scaleCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	snapshots := m.FactorSnapshots()
	result := make([]shared.FactorResult, 0, len(snapshots))
	for _, snapshot := range snapshots {
		result = append(result, shared.ToFactorResult(snapshot))
	}
	return result, nil
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
	if s.repo == nil {
		return &shared.AssessmentScaleContextResult{}, nil
	}
	var (
		medicalScale *scaledefinition.MedicalScale
		err          error
	)
	if questionnaireVersion != "" {
		// TODO: remove legacy scales fallback after questionnaire-version context fully reads published_assessment_models.
		medicalScale, err = s.repo.FindByQuestionnaireRef(ctx, questionnaireCode, questionnaireVersion)
	} else {
		// TODO: remove legacy scales fallback after questionnaire-code context has an assessment_models lookup.
		medicalScale, err = s.repo.FindPublishedByQuestionnaireCode(ctx, questionnaireCode)
	}
	if err != nil {
		if scaledefinition.IsNotFound(err) {
			return &shared.AssessmentScaleContextResult{}, nil
		}
		return nil, err
	}
	if medicalScale == nil {
		return &shared.AssessmentScaleContextResult{}, nil
	}
	scaleID := medicalScale.GetID().Uint64()
	scaleCode := medicalScale.GetCode().Value()
	scaleName := medicalScale.GetTitle()
	scaleVersion := medicalScale.GetScaleVersion()
	return &shared.AssessmentScaleContextResult{
		MedicalScaleID:   &scaleID,
		MedicalScaleCode: &scaleCode,
		MedicalScaleName: &scaleName,
		ScaleVersion:     &scaleVersion,
	}, nil
}

func (s *queryService) getScaleResultFromAssessmentModel(ctx context.Context, code string) (*shared.ScaleResult, bool, error) {
	if s == nil || s.modelRepo == nil {
		return nil, false, nil
	}
	model, err := s.modelRepo.FindByCode(ctx, code)
	if err != nil {
		if domain.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, errors.WrapC(err, errorCode.ErrDatabase, "获取测评模型失败")
	}
	if model == nil || model.Kind != domain.KindScale {
		return nil, false, nil
	}
	result, err := legacyadapter.ScaleResultFromAssessmentModel(model)
	if err != nil {
		return nil, false, errors.WrapC(err, errorCode.ErrDatabase, "解析量表模型失败")
	}
	return result, true, nil
}

func (s *queryService) getScaleResultFromPublishedModel(ctx context.Context, code string) (*shared.ScaleResult, bool, error) {
	if s == nil || s.published == nil {
		return nil, false, nil
	}
	snapshot, err := s.published.FindLatestPublishedByModelCode(ctx, domain.KindScale, code)
	if err != nil {
		if domain.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, errors.WrapC(err, errorCode.ErrDatabase, "获取已发布测评模型失败")
	}
	result, err := legacyadapter.ScaleResultFromPublishedModel(snapshot)
	if err != nil {
		return nil, false, errors.WrapC(err, errorCode.ErrDatabase, "解析已发布量表模型失败")
	}
	return result, true, nil
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
	}, true, nil
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
		parsed, ok := scaledefinition.ParseStatus(normalized.Status)
		if !ok {
			return scalereadmodel.ScaleFilter{}, errors.WithCode(errorCode.ErrInvalidArgument, "状态无效")
		}
		normalized.Status = parsed.Value()
	}
	return normalized, nil
}
