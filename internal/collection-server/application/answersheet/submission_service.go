package answersheet

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	collectionquestionnaire "github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/surveyvalidation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AssessmentResolver reads the asynchronous Assessment created by the worker.
type AssessmentResolver interface {
	ResolveAssessmentByAnswerSheetID(ctx context.Context, answerSheetID uint64) (testeeID, assessmentID uint64, err error)
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
	submitGuard        LeaseIdempotencyGuard
	assessmentResolver AssessmentResolver
	questionnaire      submissionQuestionnaireReader
	acceptTimeout      time.Duration
}

// NewSubmissionService 创建答卷提交服务
func NewSubmissionService(
	answerSheetWriter AnswerSheetWriter,
	answerSheetReader AnswerSheetReader,
	actorClient ActorLookup,
	profileLinkService profileLinkChecker,
	submitGuard LeaseIdempotencyGuard,
	assessmentResolver AssessmentResolver,
	questionnaire submissionQuestionnaireReader,
	acceptTimeout time.Duration,
) *SubmissionService {
	if acceptTimeout <= 0 {
		acceptTimeout = 2 * time.Second
	}
	return &SubmissionService{
		answerSheetWriter:  answerSheetWriter,
		answerSheetReader:  answerSheetReader,
		actorClient:        actorClient,
		profileLinkService: profileLinkService,
		profileAccess:      NewProfileAccessResolver(actorClient, profileLinkService),
		answerConverter:    AnswerConverter{},
		committer:          NewSubmissionCommitter(answerSheetWriter),
		submitGuard:        submitGuard,
		assessmentResolver: assessmentResolver,
		questionnaire:      questionnaire,
		acceptTimeout:      acceptTimeout,
	}
}

var safeIdempotencyKey = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)

func observeSubmitStage(stage, outcome string, started time.Time) {
	resilience.ObserveAnswerSheetSubmitStage(stage, outcome, time.Since(started))
}

func (s *SubmissionService) validateBeforeAccept(ctx context.Context, req *SubmitAnswerSheetRequest) error {
	if req == nil {
		return status.Error(codes.InvalidArgument, "answersheet request is required")
	}
	if !safeIdempotencyKey.MatchString(req.IdempotencyKey) {
		return status.Error(codes.InvalidArgument, "idempotency_key must contain 8-128 safe characters")
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

// AcceptDurably returns only after the apiserver has committed the AnswerSheet,
// idempotency record and outbox event in one transaction.
func (s *SubmissionService) AcceptDurably(ctx context.Context, requestID string, writerID uint64, req *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
	totalStarted := time.Now()
	totalOutcome := "rejected"
	defer func() {
		observeSubmitStage("total", totalOutcome, totalStarted)
		resilience.ObserveAnswerSheetSubmitOutcome(totalOutcome)
	}()
	ctx, cancel := context.WithTimeout(ctx, s.acceptTimeout)
	defer cancel()
	preflightStarted := time.Now()
	if err := s.validateBeforeAccept(ctx, req); err != nil {
		observeSubmitStage("preflight", "failed", preflightStarted)
		return nil, err
	}
	observeSubmitStage("preflight", "ok", preflightStarted)
	if s.submitGuard == nil {
		response, err := s.accept(ctx, requestID, writerID, req)
		if err == nil {
			totalOutcome = "accepted"
		} else if status.Code(err) == codes.AlreadyExists {
			totalOutcome = "conflict"
		} else if status.Code(err) == codes.Unavailable || status.Code(err) == codes.DeadlineExceeded {
			totalOutcome = "unavailable"
		}
		return response, err
	}
	var response *SubmitAnswerSheetResponse
	guardKey := strconv.FormatUint(writerID, 10) + ":" + req.IdempotencyKey
	_, acquired, err := s.submitGuard.Run(ctx, guardKey, func(runCtx context.Context) (string, error) {
		var acceptErr error
		response, acceptErr = s.accept(runCtx, requestID, writerID, req)
		if acceptErr != nil || response == nil {
			return "", acceptErr
		}
		return response.ID, nil
	})
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			totalOutcome = "conflict"
		} else if status.Code(err) == codes.Unavailable || status.Code(err) == codes.DeadlineExceeded {
			totalOutcome = "unavailable"
		}
		return nil, err
	}
	if !acquired {
		totalOutcome = "busy"
		return nil, status.Error(codes.ResourceExhausted, "submit already in progress")
	}
	totalOutcome = "accepted"
	return response, nil
}

func (s *SubmissionService) accept(ctx context.Context, requestID string, writerID uint64, req *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
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
	identityStarted := time.Now()
	if err := s.validateWriter(ctx, writerID); err != nil {
		observeSubmitStage("identity", "failed", identityStarted)
		return nil, err
	}
	observeSubmitStage("identity", "ok", identityStarted)

	// 2. 校验 active ProfileLink 权限，并获取 testee 信息（用于获取 OrgID）
	profileLinkStarted := time.Now()
	testee, resolvedTesteeID, err := s.profileAccess.Resolve(ctx, writerID, req.TesteeID)
	if err != nil {
		observeSubmitStage("profile_link", "failed", profileLinkStarted)
		return nil, err
	}
	observeSubmitStage("profile_link", "ok", profileLinkStarted)

	answers := s.answerConverter.Convert(req.Answers)

	// 4. 调用 gRPC 服务提交答卷（传递 OrgID）
	orgID := uint64(0)
	if testee != nil {
		orgID = testee.OrgID
	}
	grpcSaveStarted := time.Now()
	result, err := s.committer.Save(ctx, writerID, orgID, resolvedTesteeID, req, answers)
	if err != nil {
		observeSubmitStage("grpc_save", "failed", grpcSaveStarted)
		return nil, err
	}
	if result == nil || result.ID == 0 {
		observeSubmitStage("grpc_save", "failed", grpcSaveStarted)
		return nil, status.Error(codes.Unavailable, "answer sheet durable save returned no result")
	}
	observeSubmitStage("grpc_save", "ok", grpcSaveStarted)
	duration := time.Since(startTime)
	l.Infow("答卷可靠受理成功", "action", "submit_answersheet", "result", "accepted",
		"request_id", requestID,
		"idempotency_key", req.IdempotencyKey,
		"answersheet_id", result.ID,
		"duration_ms", duration.Milliseconds(),
	)

	return &SubmitAnswerSheetResponse{
		ID:      strconv.FormatUint(result.ID, 10),
		Message: result.Message,
	}, nil
}

func (s *SubmissionService) GetAssessmentReadiness(ctx context.Context, writerID, answerSheetID, requestedTesteeID uint64) (response *AssessmentReadinessResponse, returnErr error) {
	defer func() {
		readinessStatus := "error"
		if response != nil && response.Status != "" {
			readinessStatus = response.Status
		} else {
			switch status.Code(returnErr) {
			case codes.InvalidArgument:
				readinessStatus = "invalid"
			case codes.PermissionDenied:
				readinessStatus = "forbidden"
			case codes.NotFound:
				readinessStatus = "not_found"
			}
		}
		resilience.ObserveAssessmentReadiness(readinessStatus)
	}()
	if writerID == 0 || requestedTesteeID == 0 {
		return nil, status.Error(codes.InvalidArgument, "writer_id and testee_id are required")
	}
	sheet, err := s.Get(ctx, answerSheetID)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, err
		}
		return nil, status.Error(codes.Unavailable, "answer sheet readiness dependency is unavailable")
	}
	if sheet == nil {
		return nil, status.Error(codes.NotFound, "answer sheet not found")
	}
	actualTesteeID, err := strconv.ParseUint(sheet.TesteeID, 10, 64)
	if err != nil || actualTesteeID == 0 {
		return nil, status.Error(codes.Unavailable, "answer sheet ownership is unavailable")
	}
	if actualTesteeID != requestedTesteeID {
		return nil, status.Error(codes.PermissionDenied, "answer sheet does not belong to testee")
	}
	if _, resolvedID, err := s.profileAccess.Resolve(ctx, writerID, requestedTesteeID); err != nil {
		return nil, err
	} else if resolvedID != actualTesteeID {
		return nil, status.Error(codes.PermissionDenied, "answer sheet does not belong to testee")
	}
	if s.assessmentResolver == nil {
		return nil, status.Error(codes.Unavailable, "assessment readiness is unavailable")
	}
	resolvedTesteeID, assessmentID, err := s.assessmentResolver.ResolveAssessmentByAnswerSheetID(ctx, answerSheetID)
	if status.Code(err) == codes.NotFound {
		logger.L(ctx).Infow("测评尚未就绪", "action", "assessment_readiness", "stage", "assessment_pending",
			"answersheet_id", answerSheetID, "testee_id", actualTesteeID, "result", "pending")
		return &AssessmentReadinessResponse{Status: "pending", AnswerSheetID: strconv.FormatUint(answerSheetID, 10), NextPollAfterMs: 2000}, nil
	}
	if err != nil {
		return nil, status.Error(codes.Unavailable, "assessment readiness is unavailable")
	}
	if resolvedTesteeID != actualTesteeID || assessmentID == 0 {
		return nil, status.Error(codes.Unavailable, "assessment readiness is inconsistent")
	}
	if createdAt, parseErr := time.Parse(time.RFC3339Nano, sheet.CreatedAt); parseErr == nil {
		resilience.ObserveSubmitToAssessmentReady(time.Since(createdAt))
	}
	logger.L(ctx).Infow("测评已就绪", "action", "assessment_readiness", "stage", "assessment_ready",
		"answersheet_id", answerSheetID, "assessment_id", assessmentID, "testee_id", actualTesteeID, "result", "ready")
	return &AssessmentReadinessResponse{
		Status:        "ready",
		AnswerSheetID: strconv.FormatUint(answerSheetID, 10),
		AssessmentID:  strconv.FormatUint(assessmentID, 10),
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
