package questionnaire

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/pkg/cancelerr"
	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
)

type questionnaireClient = CatalogReader

// QueryService 问卷查询服务
type QueryService struct {
	client          questionnaireClient
	cache           PublishedDetailCache
	coalescer       loadguard.Coalescer
	useSingleflight bool
}

// NewQueryService 创建问卷查询服务。
func NewQueryService(
	client questionnaireClient,
	cache PublishedDetailCache,
	useSingleflight bool,
) *QueryService {
	svc := &QueryService{
		client:          client,
		cache:           cache,
		useSingleflight: useSingleflight,
	}
	if useSingleflight {
		svc.coalescer = loadguard.NewCoalescer(true)
	}
	return svc
}

// HasCachedDetail 进程内 L1 是否已有已发布问卷详情。
func (s *QueryService) HasCachedDetail(code, version string) bool {
	if s == nil || s.cache == nil || code == "" {
		return false
	}
	_, ok := s.cache.Get(code, version)
	return ok
}

// Get 获取问卷详情
func (s *QueryService) Get(ctx context.Context, code, version string) (*QuestionnaireResponse, error) {
	return s.readThroughDetail(
		cacheKey(code, version),
		func() (*QuestionnaireResponse, bool) {
			if s.cache == nil {
				return nil, false
			}
			return s.cache.Get(code, version)
		},
		func(resp *QuestionnaireResponse) { s.cache.Set(code, version, resp) },
		func() (*QuestionnaireResponse, error) { return s.fetchFromGRPC(ctx, code, version) },
	)
}

func (s *QueryService) fetchFromGRPC(ctx context.Context, code, version string) (*QuestionnaireResponse, error) {
	log.Infof("Getting questionnaire: code=%s version=%s", code, version)

	result, err := s.client.GetQuestionnaire(ctx, code, version)
	if err != nil {
		logQuestionnaireGRPCError("Failed to get questionnaire via gRPC", err)
		return nil, err
	}
	return result, nil
}

// List 获取问卷列表（返回摘要，不含问题详情）
func (s *QueryService) List(ctx context.Context, req *ListQuestionnairesRequest) (*ListQuestionnairesResponse, error) {
	log.Infof("Listing questionnaires: page=%d, pageSize=%d, status=%s", req.Page, req.PageSize, req.Status)

	// 默认分页参数
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 50
	}
	// 最大分页限制，避免一次查询过多数据
	if req.PageSize > 100 {
		req.PageSize = 100
	}

	result, err := s.client.ListQuestionnaires(ctx, req.Page, req.PageSize, req.Status, req.Title)
	if err != nil {
		logQuestionnaireGRPCError("Failed to list questionnaires via gRPC", err)
		return nil, err
	}
	return result, nil
}

func logQuestionnaireGRPCError(message string, err error) {
	if cancelerr.Is(err) {
		log.Debugf("%s: %v", message, err)
		return
	}
	log.Errorf("%s: %v", message, err)
}
