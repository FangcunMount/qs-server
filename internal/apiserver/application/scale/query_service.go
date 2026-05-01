package scale

import (
	"context"
	"strings"

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
	// 1. 验证分页参数
	if dto.Page <= 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "页码必须大于0")
	}
	if dto.PageSize <= 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "每页数量必须大于0")
	}
	if dto.PageSize > 100 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "每页数量不能超过100")
	}
	filter, err := s.normalizeScaleFilter(dto.Filter)
	if err != nil {
		return nil, err
	}

	// 2. 获取量表摘要列表
	items, err := s.reader.ListScales(ctx, filter, scalereadmodel.PageRequest{Page: dto.Page, PageSize: dto.PageSize})
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取量表列表失败")
	}

	// 3. 获取总数
	total, err := s.reader.CountScales(ctx, filter)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取量表总数失败")
	}

	return toSummaryRowsResult(ctx, items, total, s.identitySvc), nil
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

// ListPublished 查询已发布量表摘要列表
func (s *queryService) ListPublished(ctx context.Context, dto ListScalesDTO) (*ScaleSummaryListResult, error) {
	// 1. 验证分页参数
	if dto.Page <= 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "页码必须大于0")
	}
	if dto.PageSize <= 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "每页数量必须大于0")
	}
	if dto.PageSize > 100 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "每页数量不能超过100")
	}

	// 2. 添加状态过滤条件
	filter, err := s.normalizeScaleFilter(dto.Filter)
	if err != nil {
		return nil, err
	}
	filter.Status = scale.StatusPublished.Value()

	// 3. 尝试使用全量列表缓存（仅当没有额外筛选条件）
	if filter.Title == "" && filter.Category == "" && s.listCache != nil {
		if cached, ok := s.listCache.GetPage(ctx, dto.Page, dto.PageSize); ok {
			return scaleSummaryListResultFromCachePage(cached), nil
		}
	}

	// 3. 获取量表摘要列表
	items, err := s.reader.ListScales(ctx, filter, scalereadmodel.PageRequest{Page: dto.Page, PageSize: dto.PageSize})
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取量表列表失败")
	}

	// 4. 获取总数
	total, err := s.reader.CountScales(ctx, filter)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取量表总数失败")
	}

	result := toSummaryRowsResult(ctx, items, total, s.identitySvc)

	// 6. 缓存回填：仅在纯已发布列表且缓存启用时尝试重建
	if filter.Title == "" && filter.Category == "" && s.listCache != nil {
		go func() {
			_ = s.listCache.Rebuild(context.Background())
		}()
	}
	s.recordHotset(ctx, cachetarget.NewStaticScaleListWarmupTarget())

	return result, nil
}

// ListHotPublished 查询热门已发布量表摘要列表。
func (s *queryService) ListHotPublished(ctx context.Context, dto ListHotScalesDTO) (*HotScaleListResult, error) {
	limit := normalizeHotScaleLimit(dto.Limit)
	windowDays := normalizeHotScaleWindowDays(dto.WindowDays)

	hotItems, err := s.loadHotScaleRank(ctx, limit, windowDays)
	if err != nil {
		logger.L(ctx).Warnw("failed to load hot scale rank from redis",
			"window_days", windowDays,
			"limit", limit,
			"error", err,
		)
	}

	if len(hotItems) < limit {
		fallback, err := s.loadHotScaleFallback(ctx, limit, hotItems)
		if err != nil {
			return nil, err
		}
		hotItems = append(hotItems, fallback...)
	}
	if len(hotItems) > limit {
		hotItems = hotItems[:limit]
	}

	return toHotScaleListResult(ctx, hotItems, limit, windowDays, s.identitySvc), nil
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

func (s *queryService) recordHotset(ctx context.Context, target cachetarget.WarmupTarget) {
	if s == nil || s.hotset == nil {
		return
	}
	_ = s.hotset.Record(ctx, target)
}

func (s *queryService) normalizeScaleFilter(filter scalereadmodel.ScaleFilter) (scalereadmodel.ScaleFilter, error) {
	if filter.Status != "" {
		parsed, ok := scale.ParseStatus(filter.Status)
		if !ok {
			return scalereadmodel.ScaleFilter{}, errors.WithCode(errorCode.ErrInvalidArgument, "状态无效")
		}
		filter.Status = parsed.Value()
	}
	return filter, nil
}

func (s *queryService) loadHotScaleRank(ctx context.Context, limit, windowDays int) ([]scale.HotScaleSummary, error) {
	if s == nil || s.hotRank == nil {
		return []scale.HotScaleSummary{}, nil
	}
	rankItems, err := s.hotRank.Top(ctx, scale.ScaleHotRankQuery{
		WindowDays: windowDays,
		Limit:      hotRankCandidateLimit(limit),
	})
	if err != nil {
		return nil, err
	}

	result := make([]scale.HotScaleSummary, 0, limit)
	seen := make(map[string]struct{}, len(rankItems))
	for _, rankItem := range rankItems {
		questionnaireCode := strings.TrimSpace(rankItem.QuestionnaireCode)
		if questionnaireCode == "" {
			continue
		}
		item, err := s.repo.FindByQuestionnaireCode(ctx, questionnaireCode)
		if err != nil {
			logger.L(ctx).Warnw("failed to resolve hot scale by questionnaire code",
				"questionnaire_code", questionnaireCode,
				"error", err,
			)
			continue
		}
		if item == nil || !item.IsPublished() || !item.GetCategory().IsOpen() {
			continue
		}
		code := item.GetCode().String()
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, scale.HotScaleSummary{
			Scale:           item,
			SubmissionCount: rankItem.Score,
		})
		if len(result) >= limit {
			break
		}
	}

	return result, nil
}

func (s *queryService) loadHotScaleFallback(ctx context.Context, limit int, existing []scale.HotScaleSummary) ([]scale.HotScaleSummary, error) {
	seen := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		if item.Scale == nil {
			continue
		}
		seen[item.Scale.GetCode().String()] = struct{}{}
	}

	rows, err := s.reader.ListScales(ctx, scalereadmodel.ScaleFilter{Status: scale.StatusPublished.Value()}, scalereadmodel.PageRequest{Page: 1, PageSize: 100})
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取热门量表兜底列表失败")
	}

	result := make([]scale.HotScaleSummary, 0, limit-len(existing))
	for _, row := range rows {
		item, err := s.repo.FindByCode(ctx, row.Code)
		if err != nil {
			logger.L(ctx).Warnw("failed to resolve fallback hot scale",
				"scale_code", row.Code,
				"error", err,
			)
			continue
		}
		if item == nil || !item.IsPublished() || !item.GetCategory().IsOpen() {
			continue
		}
		code := item.GetCode().String()
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, scale.HotScaleSummary{Scale: item})
		if len(existing)+len(result) >= limit {
			break
		}
	}
	return result, nil
}

func hotRankCandidateLimit(limit int) int {
	candidateLimit := limit * 4
	if candidateLimit < 20 {
		return 20
	}
	if candidateLimit > 100 {
		return 100
	}
	return candidateLimit
}

func normalizeHotScaleLimit(limit int) int {
	if limit <= 0 {
		return defaultHotScaleLimit
	}
	if limit < minHotScaleLimit {
		return minHotScaleLimit
	}
	if limit > maxHotScaleLimit {
		return maxHotScaleLimit
	}
	return limit
}

func normalizeHotScaleWindowDays(windowDays int) int {
	if windowDays <= 0 {
		return defaultHotScaleWindowDays
	}
	if windowDays > maxHotScaleWindowDays {
		return maxHotScaleWindowDays
	}
	return windowDays
}
