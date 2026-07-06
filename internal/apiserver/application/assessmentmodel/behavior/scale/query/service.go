package query

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/assessmentmodel/behavior/scale/ports"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/assessmentmodel/behavior/scale/shared"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/definition/hotrank"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
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

// queryService 量表查询服务实现
// 行为者：所有用户
type queryService struct {
	repo         scaleQueryRepository
	reader       scalereadmodel.ScaleReader
	identitySvc  iambridge.IdentityResolver
	listCache    scalelistcache.PublishedListCache
	hotListCache scalelistcache.HotListCache
	hotset       cachetarget.HotsetRecorder
	hotRank      hotrank.ReadModel
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
	if s == nil || s.repo == nil || questionnaireCode == "" {
		return &shared.AssessmentScaleContextResult{}, nil
	}
	var (
		medicalScale *scaledefinition.MedicalScale
		err          error
	)
	if questionnaireVersion != "" {
		medicalScale, err = s.repo.FindByQuestionnaireRef(ctx, questionnaireCode, questionnaireVersion)
	} else {
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
