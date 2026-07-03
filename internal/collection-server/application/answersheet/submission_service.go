package answersheet

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/collection-server/port/grpcbridge"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// IdempotencyGuard protects cross-instance submit idempotency for the same request key.
type IdempotencyGuard interface {
	Begin(ctx context.Context, key string) (doneAnswerSheetID string, lease *locklease.Lease, acquired bool, err error)
	Complete(ctx context.Context, key string, lease *locklease.Lease, answerSheetID string) error
	Abort(ctx context.Context, key string, lease *locklease.Lease) error
}

type answerSheetGateway interface {
	SaveAnswerSheet(ctx context.Context, input *grpcbridge.SaveAnswerSheetInput) (*grpcbridge.SaveAnswerSheetOutput, error)
	GetAnswerSheet(ctx context.Context, id uint64) (*grpcbridge.AnswerSheetOutput, error)
}

type profileLinkChecker interface {
	IsEnabled() bool
	GetDefaultOrgID() uint64
	HasActiveProfileLink(ctx context.Context, userID, iamProfileID string) (bool, error)
}

// SubmissionService 答卷提交服务
// 作为 BFF 层的薄服务，主要职责：
// 1. 转换 REST DTO 到 gRPC 请求
// 2. 调用 apiserver 的 gRPC 服务
// 3. 转换 gRPC 响应到 REST DTO
type SubmissionService struct {
	answerSheetClient  answerSheetGateway
	answerSheetReader  AnswerSheetReader
	actorClient        ActorLookup
	profileLinkService profileLinkChecker
	profileAccess      *ProfileAccessResolver
	answerConverter    AnswerConverter
	committer          *SubmissionCommitter
	queue              *SubmitQueue
	submitGuard        IdempotencyGuard
}

// NewSubmissionService 创建答卷提交服务
func NewSubmissionService(
	answerSheetClient answerSheetGateway,
	answerSheetReader AnswerSheetReader,
	actorClient ActorLookup,
	profileLinkService profileLinkChecker,
	queueOptions *options.SubmitQueueOptions,
	submitGuard IdempotencyGuard,
) *SubmissionService {
	service := &SubmissionService{
		answerSheetClient:  answerSheetClient,
		answerSheetReader:  answerSheetReader,
		actorClient:        actorClient,
		profileLinkService: profileLinkService,
		profileAccess:      NewProfileAccessResolver(actorClient, profileLinkService),
		answerConverter:    AnswerConverter{},
		committer:          NewSubmissionCommitter(answerSheetClient),
		submitGuard:        submitGuard,
	}

	if queueOptions == nil {
		queueOptions = options.NewSubmitQueueOptions()
	}
	service.queue = NewSubmitQueue(
		queueOptions.WorkerCount,
		queueOptions.QueueSize,
		service.submitWithGuard,
	)

	return service
}

// Submit 提交答卷
// writerID 来自认证中间件解析的当前用户
func (s *SubmissionService) Submit(ctx context.Context, writerID uint64, req *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
	return s.submitWithGuard(ctx, requestKey("", req), writerID, req)
}

// SubmitQueued 提交答卷（固定走排队受理）
func (s *SubmissionService) SubmitQueued(ctx context.Context, requestID string, writerID uint64, req *SubmitAnswerSheetRequest) error {
	if s.queue == nil {
		return fmt.Errorf("submit queue not initialized")
	}

	return s.queue.Enqueue(ctx, requestID, writerID, req)
}

func requestKey(requestID string, req *SubmitAnswerSheetRequest) string {
	if req != nil && req.IdempotencyKey != "" {
		return req.IdempotencyKey
	}
	return requestID
}

func (s *SubmissionService) submitWithGuard(ctx context.Context, requestID string, writerID uint64, req *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
	key := requestKey(requestID, req)
	if key == "" || s.submitGuard == nil {
		return s.submitSync(ctx, writerID, req)
	}

	doneID, lease, acquired, err := s.submitGuard.Begin(ctx, key)
	if err != nil {
		return nil, err
	}
	if doneID != "" {
		return &SubmitAnswerSheetResponse{
			ID:      doneID,
			Message: "already submitted",
		}, nil
	}
	if !acquired {
		return nil, status.Error(codes.ResourceExhausted, "submit already in progress")
	}

	resp, submitErr := s.submitSync(ctx, writerID, req)
	if submitErr != nil {
		_ = s.submitGuard.Abort(context.Background(), key, lease)
		return nil, submitErr
	}
	if resp != nil {
		if err := s.submitGuard.Complete(context.Background(), key, lease, resp.ID); err != nil {
			return nil, err
		}
	}
	return resp, nil
}

// GetSubmitStatus 获取提交状态
func (s *SubmissionService) GetSubmitStatus(requestID string) (*SubmitStatusResponse, bool) {
	if s.queue == nil {
		return nil, false
	}

	status, ok := s.queue.GetStatus(requestID)
	if !ok {
		return nil, false
	}
	return &status, true
}

func (s *SubmissionService) SubmitQueueStatusSnapshot(now time.Time) resilienceplane.QueueSnapshot {
	if s == nil || s.queue == nil {
		return resilienceplane.QueueSnapshot{
			GeneratedAt:       now,
			Component:         "collection-server",
			Name:              "answersheet_submit",
			Strategy:          "memory_channel",
			LifecycleBoundary: "process_memory_no_drain",
		}
	}
	return s.queue.StatusSnapshot(now)
}

func (s *SubmissionService) submitSync(ctx context.Context, writerID uint64, req *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	log.Infof("Submitting answer sheet: writerID=%d, testeeID=%d, questionnaireCode=%s",
		writerID, req.TesteeID, req.QuestionnaireCode)

	l.Infow("开始提交答卷",
		"action", "submit_answersheet",
		"writer_id", writerID,
		"testee_id", req.TesteeID,
		"questionnaire_code", req.QuestionnaireCode,
		"answer_count", len(req.Answers),
	)

	// 1. 校验填写人认证
	if err := s.validateWriter(ctx, writerID); err != nil {
		return nil, err
	}

	// 2. 校验 active ProfileLink 权限，并获取 testee 信息（用于获取 OrgID）
	testee, resolvedTesteeID, err := s.profileAccess.Resolve(ctx, writerID, req.TesteeID)
	if err != nil {
		return nil, err
	}

	answers := s.answerConverter.Convert(req.Answers)

	// 4. 调用 gRPC 服务提交答卷（传递 OrgID）
	orgID := uint64(0)
	if testee != nil {
		orgID = testee.OrgID
	}
	result, err := s.committer.Save(ctx, writerID, orgID, resolvedTesteeID, req, answers)
	if err != nil {
		return nil, err
	}

	// 5. 记录成功日志
	duration := time.Since(startTime)
	l.Infow("提交答卷成功", "action", "submit_answersheet", "result", "success",
		"answersheet_id", result.ID,
		"duration_ms", duration.Milliseconds(),
	)

	return &SubmitAnswerSheetResponse{
		ID:      strconv.FormatUint(result.ID, 10),
		Message: result.Message,
	}, nil
}

// validateWriter 校验填写人认证
func (s *SubmissionService) validateWriter(ctx context.Context, writerID uint64) error {
	if writerID == 0 {
		l := logger.L(ctx)
		l.Warnw("提交答卷失败：填写人ID为空", "action", "submit_answersheet", "result", "invalid_params")
		return fmt.Errorf("用户未认证")
	}
	return nil
}

func (s *SubmissionService) Get(ctx context.Context, id uint64) (*AnswerSheetResponse, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	log.Infof("Getting answer sheet: id=%d", id)

	l.Debugw("获取答卷详情",
		"action", "get_answersheet",
		"answersheet_id", id,
	)

	if s.answerSheetReader == nil {
		return nil, fmt.Errorf("answer sheet reader is not configured")
	}
	result, err := s.answerSheetReader.GetAnswerSheet(ctx, id)
	if err != nil {
		log.Errorf("Failed to get answer sheet via gRPC: %v", err)
		l.Errorw("获取答卷失败",
			"action", "get_answersheet",
			"answersheet_id", id,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, err
	}

	duration := time.Since(startTime)
	if result != nil {
		l.Debugw("获取答卷成功",
			"action", "get_answersheet",
			"answersheet_id", id,
			"questionnaire_code", result.QuestionnaireCode,
			"answer_count", len(result.Answers),
			"duration_ms", duration.Milliseconds(),
		)
	}
	return result, nil
}
