package answersheet

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
)

// SubmissionService 答卷提交服务
// 作为 BFF 层的薄服务，主要职责：
// 1. 转换 REST DTO 到 gRPC 请求
// 2. 调用 apiserver 的 gRPC 服务
// 3. 转换 gRPC 响应到 REST DTO
type SubmissionService struct {
	answerSheetClient   *grpcclient.AnswerSheetClient
	actorClient         *grpcclient.ActorClient
	guardianshipService *iam.GuardianshipService
	queue               *SubmitQueue
}

// NewSubmissionService 创建答卷提交服务
func NewSubmissionService(
	answerSheetClient *grpcclient.AnswerSheetClient,
	actorClient *grpcclient.ActorClient,
	guardianshipService *iam.GuardianshipService,
	queueOptions *options.SubmitQueueOptions,
) *SubmissionService {
	service := &SubmissionService{
		answerSheetClient:   answerSheetClient,
		actorClient:         actorClient,
		guardianshipService: guardianshipService,
	}

	if queueOptions != nil && queueOptions.Enabled {
		service.queue = NewSubmitQueue(
			queueOptions.WorkerCount,
			queueOptions.QueueSize,
			time.Duration(queueOptions.WaitTimeoutMs)*time.Millisecond,
			service.submitSync,
		)
	}

	return service
}

// Submit 提交答卷
// writerID 来自认证中间件解析的当前用户
func (s *SubmissionService) Submit(ctx context.Context, writerID uint64, req *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
	return s.submitSync(ctx, writerID, req)
}

// SubmitQueued 提交答卷（带排队）
func (s *SubmissionService) SubmitQueued(ctx context.Context, requestID string, writerID uint64, req *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, bool, error) {
	if s.queue == nil {
		resp, err := s.submitSync(ctx, writerID, req)
		return resp, false, err
	}

	return s.queue.Enqueue(ctx, requestID, writerID, req)
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

	// 2. 校验监护关系权限，并获取 testee 信息（用于获取 OrgID）
	testee, err := s.validateGuardianship(ctx, writerID, req.TesteeID)
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
	result, err := s.callSaveAnswerSheet(ctx, writerID, orgID, req, answers)
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

// validateGuardianship 校验监护关系权限，返回 testee 信息（用于获取 OrgID）
func (s *SubmissionService) validateGuardianship(ctx context.Context, writerID, testeeID uint64) (*grpcclient.TesteeResponse, error) {
	l := logger.L(ctx)

	// 查询受试者信息（需要获取 OrgID 和 IAMChildID）
	testee, err := s.actorClient.GetTestee(ctx, testeeID)
	if err != nil {
		l.Errorw("查询受试者信息失败",
			"action", "submit_answersheet",
			"testee_id", testeeID,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("查询受试者信息失败: %w", err)
	}

	// 如果 IAM 服务未启用，直接返回 testee
	if s.guardianshipService == nil || !s.guardianshipService.IsEnabled() {
		return testee, nil
	}

	// 如果受试者未绑定 IAM 用户，跳过权限校验
	if testee.IAMChildID == "" {
		l.Warnw("受试者未绑定IAM用户，跳过权限校验",
			"testee_id", testeeID,
			"testee_name", testee.Name,
		)
		return testee, nil
	}

	// 验证监护关系
	if err := s.checkGuardianRelation(ctx, writerID, testeeID, testee.IAMChildID, testee.Name); err != nil {
		return nil, err
	}

	return testee, nil
}

// checkGuardianRelation 检查监护关系
func (s *SubmissionService) checkGuardianRelation(ctx context.Context, writerID, testeeID uint64, iamChildID, testeeName string) error {
	l := logger.L(ctx)

	userIDStr := strconv.FormatUint(writerID, 10)
	isGuardian, err := s.guardianshipService.IsGuardian(ctx, userIDStr, iamChildID)
	if err != nil {
		l.Errorw("校验监护关系失败",
			"action", "submit_answersheet",
			"writer_id", writerID,
			"testee_id", testeeID,
			"iam_child_id", iamChildID,
			"error", err.Error(),
		)
		return fmt.Errorf("校验监护关系失败: %w", err)
	}

	if !isGuardian {
		l.Warnw("无权为该受试者提交答卷：不是监护人",
			"action", "submit_answersheet",
			"writer_id", writerID,
			"testee_id", testeeID,
			"iam_child_id", iamChildID,
			"testee_name", testeeName,
			"result", "forbidden",
		)
		return fmt.Errorf("无权为该受试者提交答卷")
	}

	l.Infow("监护关系验证通过",
		"action", "submit_answersheet",
		"writer_id", writerID,
		"testee_id", testeeID,
		"iam_child_id", iamChildID,
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
func (s *SubmissionService) callSaveAnswerSheet(ctx context.Context, writerID, orgID uint64, req *SubmitAnswerSheetRequest, answers []grpcclient.AnswerInput) (*grpcclient.SaveAnswerSheetOutput, error) {
	l := logger.L(ctx)

	l.Debugw("调用 gRPC 服务提交答卷",
		"questionnaire_code", req.QuestionnaireCode,
		"testee_id", req.TesteeID,
		"org_id", orgID,
	)

	result, err := s.answerSheetClient.SaveAnswerSheet(ctx, &grpcclient.SaveAnswerSheetInput{
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
		Title:                req.Title,
		WriterID:             writerID,
		TesteeID:             req.TesteeID,
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
