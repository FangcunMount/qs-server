package answersheet

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	collectionquestionnaire "github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"github.com/FangcunMount/qs-server/internal/pkg/surveyvalidation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type EnsureAssessmentInput struct {
	OrgID                uint64
	AnswerSheetID        uint64
	QuestionnaireCode    string
	QuestionnaireVersion string
	TesteeID             uint64
	FillerID             uint64
	TaskID               string
}

// SubmitAssessmentIntake synchronously advances accepted submission work to a durable Assessment.
type SubmitAssessmentIntake interface {
	EnsureAssessment(ctx context.Context, input EnsureAssessmentInput) (assessmentID uint64, err error)
	ResolveAssessmentByAnswerSheetID(ctx context.Context, answerSheetID uint64) (testeeID, assessmentID uint64, err error)
}

// IdempotencyGuard protects cross-instance submit idempotency for the same request key.
type IdempotencyGuard interface {
	Begin(ctx context.Context, key string) (doneAnswerSheetID string, lease *locklease.Lease, acquired bool, err error)
	Complete(ctx context.Context, key string, lease *locklease.Lease, answerSheetID string) error
	Abort(ctx context.Context, key string, lease *locklease.Lease) error
}

type LeaseIdempotencyGuard interface {
	Run(ctx context.Context, key string, body func(context.Context) (answerSheetID string, err error)) (doneAnswerSheetID string, acquired bool, err error)
}

type profileLinkChecker interface {
	IsEnabled() bool
	GetDefaultOrgID() uint64
	HasActiveProfileLink(ctx context.Context, userID, iamProfileID string) (bool, error)
}

// submissionQuestionnaireReader supplies the exact published version used for
// synchronous BFF preflight. QueryService provides this through its existing L1.
type submissionQuestionnaireReader interface {
	Get(ctx context.Context, code, version string) (*collectionquestionnaire.QuestionnaireResponse, error)
}

// SubmissionService 答卷提交服务
// 作为 BFF 层的薄服务，主要职责：
// 1. 转换 REST DTO 到 gRPC 请求
// 2. 调用 apiserver 的 gRPC 服务
// 3. 转换 gRPC 响应到 REST DTO
type SubmissionService struct {
	answerSheetWriter  AnswerSheetWriter
	answerSheetReader  AnswerSheetReader
	actorClient        ActorLookup
	profileLinkService profileLinkChecker
	profileAccess      *ProfileAccessResolver
	answerConverter    AnswerConverter
	committer          *SubmissionCommitter
	queue              *SubmitQueue
	submitGuard        IdempotencyGuard
	assessmentIntake   SubmitAssessmentIntake
	questionnaire      submissionQuestionnaireReader
}

// NewSubmissionService 创建答卷提交服务
func NewSubmissionService(
	answerSheetWriter AnswerSheetWriter,
	answerSheetReader AnswerSheetReader,
	actorClient ActorLookup,
	profileLinkService profileLinkChecker,
	queueOptions *options.SubmitQueueOptions,
	submitGuard IdempotencyGuard,
	assessmentIntake SubmitAssessmentIntake,
	questionnaire submissionQuestionnaireReader,
) *SubmissionService {
	service := &SubmissionService{
		answerSheetWriter:  answerSheetWriter,
		answerSheetReader:  answerSheetReader,
		actorClient:        actorClient,
		profileLinkService: profileLinkService,
		profileAccess:      NewProfileAccessResolver(actorClient, profileLinkService),
		answerConverter:    AnswerConverter{},
		committer:          NewSubmissionCommitter(answerSheetWriter),
		submitGuard:        submitGuard,
		assessmentIntake:   assessmentIntake,
		questionnaire:      questionnaire,
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
	if err := s.validateBeforeQueue(ctx, req); err != nil {
		return err
	}

	return s.queue.Enqueue(ctx, requestID, writerID, req)
}

func (s *SubmissionService) validateBeforeQueue(ctx context.Context, req *SubmitAnswerSheetRequest) error {
	if req == nil {
		return status.Error(codes.InvalidArgument, "answersheet request is required")
	}
	if s.questionnaire == nil {
		return status.Error(codes.Unavailable, "questionnaire validation is unavailable")
	}
	qnr, err := s.questionnaire.Get(ctx, req.QuestionnaireCode, req.QuestionnaireVersion)
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			return status.Error(codes.InvalidArgument, "只能提交已发布的问卷版本")
		}
		return status.Error(codes.Unavailable, "questionnaire validation is unavailable")
	}
	if qnr == nil || qnr.Code != req.QuestionnaireCode || qnr.Version != req.QuestionnaireVersion || qnr.Status != "published" {
		return status.Error(codes.InvalidArgument, "只能提交已发布的问卷版本")
	}
	answers := make([]surveyvalidation.Answer, 0, len(req.Answers))
	for _, answer := range req.Answers {
		value, err := surveyvalidation.DecodeAnswerValue(answer.QuestionType, answer.Value)
		if err != nil {
			return status.Error(codes.InvalidArgument, fmt.Sprintf("问题 %s 的答案格式不正确: %v", answer.QuestionCode, err))
		}
		answers = append(answers, surveyvalidation.Answer{QuestionCode: answer.QuestionCode, QuestionType: answer.QuestionType, Value: value})
	}
	if _, err := questionnaireSubmissionSpec(qnr).Validate(answers); err != nil {
		if validationErr, ok := err.(*surveyvalidation.Error); ok && validationErr.Kind != surveyvalidation.ErrorInvalidInput {
			log.Errorf("已发布问卷包含不可执行的提交校验配置: code=%s version=%s error=%v", qnr.Code, qnr.Version, validationErr)
			return status.Error(codes.Unavailable, "questionnaire validation is unavailable")
		}
		return status.Error(codes.InvalidArgument, fmt.Sprintf("提交答案不符合问卷规格: %v", err))
	}
	return nil
}

func questionnaireSubmissionSpec(qnr *collectionquestionnaire.QuestionnaireResponse) surveyvalidation.Spec {
	questions := make([]surveyvalidation.Question, 0, len(qnr.Questions))
	for _, question := range qnr.Questions {
		optionCodes := make([]string, 0, len(question.Options))
		for _, option := range question.Options {
			optionCodes = append(optionCodes, option.Code)
		}
		rules := make([]surveyvalidation.Rule, 0, len(question.ValidationRules))
		for _, rule := range question.ValidationRules {
			rules = append(rules, surveyvalidation.Rule{Type: rule.RuleType, TargetValue: rule.TargetValue})
		}
		var controller *surveyvalidation.ShowController
		if question.ShowController != nil {
			conditions := make([]surveyvalidation.ShowCondition, 0, len(question.ShowController.Conditions))
			for _, condition := range question.ShowController.Conditions {
				conditions = append(conditions, surveyvalidation.ShowCondition{QuestionCode: condition.QuestionCode, OptionCodes: append([]string(nil), condition.OptionCodes...)})
			}
			controller = &surveyvalidation.ShowController{Rule: question.ShowController.Rule, Conditions: conditions}
		}
		questions = append(questions, surveyvalidation.Question{Code: question.Code, Type: question.Type, OptionCodes: optionCodes, Rules: rules, ShowController: controller})
	}
	return surveyvalidation.Spec{QuestionnaireCode: qnr.Code, QuestionnaireVersion: qnr.Version, Questions: questions}
}

func requestKey(requestID string, req *SubmitAnswerSheetRequest) string {
	if req != nil && req.IdempotencyKey != "" {
		return req.IdempotencyKey
	}
	return requestID
}

func (s *SubmissionService) submitWithGuard(ctx context.Context, requestID string, writerID uint64, req *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
	key := requestKey(requestID, req)
	l := logger.L(ctx)
	if key == "" || s.submitGuard == nil {
		return s.submitSync(ctx, writerID, req)
	}
	req = withEffectiveIdempotencyKey(req, key)
	if leaseGuard, ok := s.submitGuard.(LeaseIdempotencyGuard); ok {
		return s.submitWithLeaseGuard(ctx, requestID, key, writerID, req, leaseGuard)
	}

	doneID, lease, acquired, err := s.submitGuard.Begin(ctx, key)
	if err != nil {
		return nil, err
	}
	if doneID != "" {
		assessmentID := ""
		if answerSheetID, parseErr := strconv.ParseUint(doneID, 10, 64); parseErr == nil && s.assessmentIntake != nil {
			_, id, resolveErr := s.assessmentIntake.ResolveAssessmentByAnswerSheetID(ctx, answerSheetID)
			if resolveErr != nil {
				return nil, resolveErr
			}
			if id != 0 {
				assessmentID = strconv.FormatUint(id, 10)
			}
		}
		if assessmentID == "" {
			return nil, fmt.Errorf("completed answer sheet is missing assessment id")
		}
		l.Infow("答卷提交命中幂等结果",
			"action", "submit_answersheet",
			"request_id", requestID,
			"idempotency_key", key,
			"answersheet_id", doneID,
			"assessment_id", assessmentID,
			"result", "idempotent_hit",
		)
		return &SubmitAnswerSheetResponse{
			ID:           doneID,
			AssessmentID: assessmentID,
			Message:      "already submitted",
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
		if s.queue != nil && resp.AssessmentID != "" {
			s.queue.setAssessmentID(requestID, resp.AssessmentID)
		}
		if err := s.submitGuard.Complete(context.Background(), key, lease, resp.ID); err != nil {
			return nil, err
		}
		l.Infow("答卷提交链路已完成受理",
			"action", "submit_answersheet",
			"request_id", requestID,
			"idempotency_key", key,
			"answersheet_id", resp.ID,
			"assessment_id", resp.AssessmentID,
			"result", "success",
		)
	}
	return resp, nil
}

func (s *SubmissionService) submitWithLeaseGuard(
	ctx context.Context,
	requestID string,
	key string,
	writerID uint64,
	req *SubmitAnswerSheetRequest,
	guard LeaseIdempotencyGuard,
) (*SubmitAnswerSheetResponse, error) {
	var response *SubmitAnswerSheetResponse
	doneID, acquired, err := guard.Run(ctx, key, func(runCtx context.Context) (string, error) {
		var submitErr error
		response, submitErr = s.submitSync(runCtx, writerID, req)
		if submitErr != nil || response == nil {
			return "", submitErr
		}
		return response.ID, nil
	})
	if errors.Is(err, locklease.ErrLeaseLost) || errors.Is(err, locklease.ErrLeaseRenewFailed) {
		return nil, retryableLeaseFailure{cause: err}
	}
	if err != nil {
		return nil, err
	}
	if !acquired {
		if doneID == "" {
			return nil, status.Error(codes.ResourceExhausted, "submit already in progress")
		}
		return s.completedSubmitResponse(ctx, requestID, key, doneID)
	}
	if response != nil {
		if s.queue != nil && response.AssessmentID != "" {
			s.queue.setAssessmentID(requestID, response.AssessmentID)
		}
		logger.L(ctx).Infow("答卷提交链路已完成受理",
			"action", "submit_answersheet",
			"request_id", requestID,
			"idempotency_key", key,
			"answersheet_id", response.ID,
			"assessment_id", response.AssessmentID,
			"result", "success",
		)
	}
	return response, nil
}

type retryableLeaseFailure struct{ cause error }

func (e retryableLeaseFailure) Error() string {
	return "submit lease was lost; retry with the same idempotency key: " + e.cause.Error()
}

func (e retryableLeaseFailure) Unwrap() error { return e.cause }

func (e retryableLeaseFailure) GRPCStatus() *status.Status {
	return status.New(codes.Unavailable, "submit lease was lost; retry with the same idempotency key")
}

func (s *SubmissionService) completedSubmitResponse(ctx context.Context, requestID, key, doneID string) (*SubmitAnswerSheetResponse, error) {
	assessmentID := ""
	if answerSheetID, parseErr := strconv.ParseUint(doneID, 10, 64); parseErr == nil && s.assessmentIntake != nil {
		_, id, resolveErr := s.assessmentIntake.ResolveAssessmentByAnswerSheetID(ctx, answerSheetID)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if id != 0 {
			assessmentID = strconv.FormatUint(id, 10)
		}
	}
	if assessmentID == "" {
		return nil, fmt.Errorf("completed answer sheet is missing assessment id")
	}
	logger.L(ctx).Infow("答卷提交命中幂等结果",
		"action", "submit_answersheet",
		"request_id", requestID,
		"idempotency_key", key,
		"answersheet_id", doneID,
		"assessment_id", assessmentID,
		"result", "idempotent_hit",
	)
	return &SubmitAnswerSheetResponse{ID: doneID, AssessmentID: assessmentID, Message: "already submitted"}, nil
}

func withEffectiveIdempotencyKey(req *SubmitAnswerSheetRequest, key string) *SubmitAnswerSheetRequest {
	if req == nil || key == "" || req.IdempotencyKey == key {
		return req
	}
	cloned := *req
	cloned.IdempotencyKey = key
	return &cloned
}

// GetSubmitStatus 获取提交状态。done 必须已经同时持久化两个 ID。
func (s *SubmissionService) GetSubmitStatus(ctx context.Context, requestID string) (*SubmitStatusResponse, bool) {
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
	l.Infow("答卷已持久化，开始确保测评",
		"action", "submit_answersheet",
		"answersheet_id", result.ID,
		"org_id", orgID,
		"testee_id", resolvedTesteeID,
		"questionnaire_code", req.QuestionnaireCode,
	)

	if s.assessmentIntake == nil {
		return nil, fmt.Errorf("assessment intake not configured")
	}
	assessmentID, err := s.assessmentIntake.EnsureAssessment(ctx, EnsureAssessmentInput{
		OrgID: orgID, AnswerSheetID: result.ID, QuestionnaireCode: req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion, TesteeID: resolvedTesteeID,
		FillerID: writerID, TaskID: req.TaskID,
	})
	if err != nil {
		return nil, err
	}
	if assessmentID == 0 {
		return nil, fmt.Errorf("assessment intake returned empty assessment id")
	}
	l.Infow("测评已确保，等待评估事件处理",
		"action", "submit_answersheet",
		"answersheet_id", result.ID,
		"assessment_id", assessmentID,
		"org_id", orgID,
		"testee_id", resolvedTesteeID,
	)

	// 5. 记录成功日志
	duration := time.Since(startTime)
	l.Infow("提交答卷成功", "action", "submit_answersheet", "result", "success",
		"answersheet_id", result.ID,
		"assessment_id", assessmentID,
		"duration_ms", duration.Milliseconds(),
	)

	return &SubmitAnswerSheetResponse{
		ID:           strconv.FormatUint(result.ID, 10),
		AssessmentID: strconv.FormatUint(assessmentID, 10),
		Message:      result.Message,
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
