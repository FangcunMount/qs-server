package scale

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
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
	repo        scale.Repository
	reader      scalereadmodel.ScaleReader
	identitySvc iambridge.IdentityResolver
	listCache   scalelistcache.PublishedListCache
	hotset      cachetarget.HotsetRecorder
	hotRank     scale.ScaleHotRankReadModel
}

// NewQueryService 创建量表查询服务。
func NewQueryService(repo scale.Repository, reader scalereadmodel.ScaleReader, identitySvc iambridge.IdentityResolver, listCache scalelistcache.PublishedListCache, hotset cachetarget.HotsetRecorder, hotRankReaders ...scale.ScaleHotRankReadModel) ScaleQueryService {
	return newQueryService(repo, reader, identitySvc, listCache, hotset, hotRankReaders...)
}

// NewQueryServiceWithReadModel 创建使用显式 read model 的量表查询服务。
func NewQueryServiceWithReadModel(repo scale.Repository, reader scalereadmodel.ScaleReader, identitySvc iambridge.IdentityResolver, listCache scalelistcache.PublishedListCache, hotset cachetarget.HotsetRecorder, hotRankReaders ...scale.ScaleHotRankReadModel) ScaleQueryService {
	return newQueryService(repo, reader, identitySvc, listCache, hotset, hotRankReaders...)
}

func newQueryService(repo scale.Repository, reader scalereadmodel.ScaleReader, identitySvc iambridge.IdentityResolver, listCache scalelistcache.PublishedListCache, hotset cachetarget.HotsetRecorder, hotRankReaders ...scale.ScaleHotRankReadModel) ScaleQueryService {
	var hotRank scale.ScaleHotRankReadModel
	if len(hotRankReaders) > 0 {
		hotRank = hotRankReaders[0]
	}
	return &queryService{
		repo:        repo,
		reader:      reader,
		identitySvc: identitySvc,
		listCache:   listCache,
		hotset:      hotset,
		hotRank:     hotRank,
	}
}

// GetByCode 根据编码获取量表
func (s *queryService) GetByCode(ctx context.Context, code string) (*ScaleResult, error) {
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

	return toScaleResultWithUsers(ctx, m, s.identitySvc), nil
}

// GetByQuestionnaireCode 根据问卷编码获取量表
func (s *queryService) GetByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*ScaleResult, error) {
	// 1. 验证输入参数
	if questionnaireCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "问卷编码不能为空")
	}

	// 2. 从仓储获取量表
	m, err := s.repo.FindByQuestionnaireCode(ctx, questionnaireCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	return toScaleResultWithUsers(ctx, m, s.identitySvc), nil
}

// List 查询量表摘要列表
func (s *queryService) List(ctx context.Context, dto ListScalesDTO) (*ScaleSummaryListResult, error) {
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
func (s *queryService) GetPublishedByCode(ctx context.Context, code string) (*ScaleResult, error) {
	// 1. 验证输入参数
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	// 2. 获取量表
	m, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	// 3. 检查量表状态
	if !m.IsPublished() {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表未发布")
	}
	s.recordHotset(ctx, cachetarget.NewStaticScaleWarmupTarget(code))

	return toScaleResultWithUsers(ctx, m, s.identitySvc), nil
}

// GetFactors 获取量表的因子列表
func (s *queryService) GetFactors(ctx context.Context, scaleCode string) ([]FactorResult, error) {
	// 1. 验证输入参数
	if scaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	// 2. 从仓储获取量表
	m, err := s.repo.FindByCode(ctx, scaleCode)
	logger.L(ctx).Infow("GetFactors: 获取量表", "scaleCode", scaleCode, "err", err)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	// 3. 转换因子列表
	factors := m.GetFactors()
	logger.L(ctx).Infow("GetFactors: 获取因子列表", "factors", factors)
	result := make([]FactorResult, 0, len(factors))
	for _, factor := range factors {
		result = append(result, toFactorResult(factor))
		logger.L(ctx).Infow("GetFactors: 转换因子列表", "factor", factor, "result", result)
	}
	logger.L(ctx).Infow("GetFactors: 转换因子列表", "result", result)
	return result, nil
}

// ResolveAssessmentScaleContext 按问卷编码解析创建测评所需的量表上下文。
func (s *queryService) ResolveAssessmentScaleContext(ctx context.Context, questionnaireCode string) (*AssessmentScaleContextResult, error) {
	if s == nil || s.repo == nil || questionnaireCode == "" {
		return &AssessmentScaleContextResult{}, nil
	}
	medicalScale, err := s.repo.FindByQuestionnaireCode(ctx, questionnaireCode)
	if err != nil || medicalScale == nil {
		if err != nil && !scale.IsNotFound(err) {
			logger.L(ctx).Infow("问卷未关联量表，将创建纯问卷模式的测评",
				"questionnaire_code", questionnaireCode,
				"error", err,
			)
		}
		return &AssessmentScaleContextResult{}, nil
	}
	scaleID := medicalScale.GetID().Uint64()
	scaleCode := medicalScale.GetCode().Value()
	scaleName := medicalScale.GetTitle()
	return &AssessmentScaleContextResult{
		MedicalScaleID:   &scaleID,
		MedicalScaleCode: &scaleCode,
		MedicalScaleName: &scaleName,
	}, nil
}

func (s *queryService) recordHotset(ctx context.Context, target cachetarget.WarmupTarget) {
	if s == nil || s.hotset == nil {
		return
	}
	_ = s.hotset.Record(ctx, target)
}

func (s *queryService) normalizeScaleFilter(filter ScaleListFilter) (scalereadmodel.ScaleFilter, error) {
	normalized := scalereadmodel.ScaleFilter{
		Status:   filter.Status,
		Title:    filter.Title,
		Category: filter.Category,
	}
	if normalized.Status != "" {
		parsed, ok := scale.ParseStatus(normalized.Status)
		if !ok {
			return scalereadmodel.ScaleFilter{}, errors.WithCode(errorCode.ErrInvalidArgument, "状态无效")
		}
		normalized.Status = parsed.Value()
	}
	return normalized, nil
}
