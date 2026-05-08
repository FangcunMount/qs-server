package answersheet

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type actorLookupClient interface {
	GetTestee(ctx context.Context, testeeID uint64) (*grpcclient.TesteeResponse, error)
	TesteeExists(ctx context.Context, orgID, iamProfileID uint64) (exists bool, testeeID uint64, err error)
}

// IdempotencyGuard protects cross-instance submit idempotency for the same request key.
type IdempotencyGuard interface {
	Begin(ctx context.Context, key string) (doneAnswerSheetID string, lease *locklease.Lease, acquired bool, err error)
	Complete(ctx context.Context, key string, lease *locklease.Lease, answerSheetID string) error
	Abort(ctx context.Context, key string, lease *locklease.Lease) error
}

type answerSheetGateway interface {
	SaveAnswerSheet(ctx context.Context, input *grpcclient.SaveAnswerSheetInput) (*grpcclient.SaveAnswerSheetOutput, error)
	GetAnswerSheet(ctx context.Context, id uint64) (*grpcclient.AnswerSheetOutput, error)
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
	actorClient        actorLookupClient
	profileLinkService profileLinkChecker
	queue              *SubmitQueue
	submitGuard        IdempotencyGuard
}

// NewSubmissionService 创建答卷提交服务
func NewSubmissionService(
	answerSheetClient answerSheetGateway,
	actorClient actorLookupClient,
	profileLinkService profileLinkChecker,
	queueOptions *options.SubmitQueueOptions,
	submitGuard IdempotencyGuard,
) *SubmissionService {
	service := &SubmissionService{
		answerSheetClient:  answerSheetClient,
		actorClient:        actorClient,
		profileLinkService: profileLinkService,
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
	testee, resolvedTesteeID, err := s.validateProfileAccess(ctx, writerID, req.TesteeID)
	if err != nil {
		return nil, err
	}

	// 3. 转换答案数据
	answers := s.convertAnswers(req.Answers)

	// 4. 调用 gRPC 服务提交答卷（传递 OrgID）
	orgID := uint64(0)
	if testee != nil {
		orgID = testee.OrgID
	}
	result, err := s.callSaveAnswerSheet(ctx, writerID, orgID, resolvedTesteeID, req, answers)
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

// validateProfileAccess verifies active ProfileLink access and returns canonical testee data.
func (s *SubmissionService) validateProfileAccess(ctx context.Context, writerID, testeeID uint64) (*grpcclient.TesteeResponse, uint64, error) {
	l := logger.L(ctx)

	// 查询受试者信息（需要获取 OrgID 和 IAMProfileID）。
	// 兼容某些上游把 profile_id 误传成 testee_id 的情况。
	testee, resolvedTesteeID, err := s.resolveCanonicalTestee(ctx, testeeID)
	if err != nil {
		l.Errorw("查询受试者信息失败",
			"action", "submit_answersheet",
			"testee_id", testeeID,
			"error", err.Error(),
		)
		return nil, 0, err
	}

	// 如果 IAM 服务未启用，直接返回 testee
	if s.profileLinkService == nil || !s.profileLinkService.IsEnabled() {
		return testee, resolvedTesteeID, nil
	}

	// 如果受试者未绑定 IAM Profile，跳过权限校验
	if testee.IAMProfileID == "" {
		l.Warnw("受试者未绑定 IAM Profile，跳过权限校验",
			"testee_id", resolvedTesteeID,
			"testee_name", testee.Name,
		)
		return testee, resolvedTesteeID, nil
	}

	// 验证 active ProfileLink
	if err := s.checkProfileLinkAccess(ctx, writerID, resolvedTesteeID, testee.IAMProfileID, testee.Name); err != nil {
		return nil, 0, err
	}

	return testee, resolvedTesteeID, nil
}

func (s *SubmissionService) resolveCanonicalTestee(ctx context.Context, rawTesteeID uint64) (*grpcclient.TesteeResponse, uint64, error) {
	testee, err := s.actorClient.GetTestee(ctx, rawTesteeID)
	if err == nil {
		return testee, rawTesteeID, nil
	}
	if status.Code(err) != codes.NotFound || s.profileLinkService == nil {
		return nil, 0, fmt.Errorf("查询受试者信息失败: %w", err)
	}

	orgID := s.profileLinkService.GetDefaultOrgID()
	exists, canonicalTesteeID, existsErr := s.actorClient.TesteeExists(ctx, orgID, rawTesteeID)
	if existsErr != nil {
		return nil, 0, fmt.Errorf("查询受试者信息失败: %w", err)
	}
	if !exists || canonicalTesteeID == 0 {
		return nil, 0, fmt.Errorf("查询受试者信息失败: %w", err)
	}

	canonicalTestee, canonicalErr := s.actorClient.GetTestee(ctx, canonicalTesteeID)
	if canonicalErr != nil {
		return nil, 0, fmt.Errorf("查询受试者信息失败: %w", canonicalErr)
	}

	logger.L(ctx).Warnw("提交答卷时检测到 profile_id 被误作 testee_id，已自动回退到 canonical testee_id",
		"action", "submit_answersheet",
		"submitted_testee_id", rawTesteeID,
		"canonical_testee_id", canonicalTesteeID,
		"org_id", orgID,
	)
	return canonicalTestee, canonicalTesteeID, nil
}

// checkProfileLinkAccess checks whether writerID has an active ProfileLink to iamProfileID.
func (s *SubmissionService) checkProfileLinkAccess(ctx context.Context, writerID, testeeID uint64, iamProfileID, testeeName string) error {
	l := logger.L(ctx)

	userIDStr := strconv.FormatUint(writerID, 10)
	hasActiveProfileLink, err := s.profileLinkService.HasActiveProfileLink(ctx, userIDStr, iamProfileID)
	if err != nil {
		l.Errorw("校验 ProfileLink 权限失败",
			"action", "submit_answersheet",
			"writer_id", writerID,
			"testee_id", testeeID,
			"iam_profile_id", iamProfileID,
			"error", err.Error(),
		)
		return fmt.Errorf("校验 ProfileLink 权限失败: %w", err)
	}

	if !hasActiveProfileLink {
		l.Warnw("无权为该受试者提交答卷：缺少 active ProfileLink",
			"action", "submit_answersheet",
			"writer_id", writerID,
			"testee_id", testeeID,
			"iam_profile_id", iamProfileID,
			"testee_name", testeeName,
			"result", "forbidden",
		)
		return fmt.Errorf("无权为该受试者提交答卷")
	}

	l.Infow("ProfileLink 权限验证通过",
		"action", "submit_answersheet",
		"writer_id", writerID,
		"testee_id", testeeID,
		"iam_profile_id", iamProfileID,
	)
	return nil
}

// convertAnswers 转换答案数据
func (s *SubmissionService) convertAnswers(answers []Answer) []grpcclient.AnswerInput {
	result := make([]grpcclient.AnswerInput, len(answers))
	for i, a := range answers {
		result[i] = grpcclient.AnswerInput{
			QuestionCode: a.QuestionCode,
			QuestionType: a.QuestionType,
			Score:        a.Score,
			Value:        a.Value,
		}
	}
	return result
}

// callSaveAnswerSheet 调用 gRPC 服务保存答卷
func (s *SubmissionService) callSaveAnswerSheet(ctx context.Context, writerID, orgID, testeeID uint64, req *SubmitAnswerSheetRequest, answers []grpcclient.AnswerInput) (*grpcclient.SaveAnswerSheetOutput, error) {
	l := logger.L(ctx)

	l.Debugw("调用 gRPC 服务提交答卷",
		"questionnaire_code", req.QuestionnaireCode,
		"testee_id", testeeID,
		"org_id", orgID,
	)

	result, err := s.answerSheetClient.SaveAnswerSheet(ctx, &grpcclient.SaveAnswerSheetInput{
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
		IdempotencyKey:       req.IdempotencyKey,
		Title:                req.Title,
		WriterID:             writerID,
		TesteeID:             testeeID,
		TaskID:               req.TaskID,
		OrgID:                orgID,
		Answers:              answers,
	})
	if err != nil {
		log.Errorf("Failed to save answer sheet via gRPC: %v", err)
		l.Errorw("提交答卷失败",
			"action", "submit_answersheet",
			"questionnaire_code", req.QuestionnaireCode,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, err
	}

	return result, nil
}

// Get 获取答卷详情
func (s *SubmissionService) Get(ctx context.Context, id uint64) (*AnswerSheetResponse, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	log.Infof("Getting answer sheet: id=%d", id)

	l.Debugw("获取答卷详情",
		"action", "get_answersheet",
		"answersheet_id", id,
	)

	result, err := s.answerSheetClient.GetAnswerSheet(ctx, id)
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

	// 转换 answers
	answers := make([]Answer, len(result.Answers))
	for i, a := range result.Answers {
		answers[i] = Answer{
			QuestionCode: a.QuestionCode,
			QuestionType: a.QuestionType,
			Score:        a.Score,
			Value:        a.Value,
		}
	}

	duration := time.Since(startTime)
	l.Debugw("获取答卷成功",
		"action", "get_answersheet",
		"answersheet_id", id,
		"questionnaire_code", result.QuestionnaireCode,
		"answer_count", len(answers),
		"duration_ms", duration.Milliseconds(),
	)

	return &AnswerSheetResponse{
		ID:                   strconv.FormatUint(result.ID, 10),
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		Title:                result.Title,
		Score:                result.Score,
		WriterID:             strconv.FormatUint(result.WriterID, 10),
		WriterName:           result.WriterName,
		TesteeID:             strconv.FormatUint(result.TesteeID, 10),
		TesteeName:           result.TesteeName,
		Answers:              answers,
		CreatedAt:            result.CreatedAt,
		UpdatedAt:            result.UpdatedAt,
	}, nil
}
